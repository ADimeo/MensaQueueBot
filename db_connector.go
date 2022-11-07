/*Implements database logic related to storing and retrieving actual queue length reports
 */
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

var globalPseudonymizationAttribute pseudonymizationAttribute

type pseudonymizationAttribute struct {
	Timestamp time.Time
	Random    string // 32 bytes of random, defined in getReporterPepper
}

/*Regenerates the global pseudonymization attribute. Fails silently if random
  can't be accessed for some reason.
*/

func initializeNewPseudonymizationAttribute(generationTime time.Time) {
	zap.S().Info("Regenerating attribute for pseudonymisation")
	newRandom, err := GenerateRandomString(32)
	if err == nil {
		// If we get an error that means we don't have enough random. Keep reusing old pepper, and hope random comes back.
		// But really, this shouldn't happen.
		globalPseudonymizationAttribute.Random = newRandom
		globalPseudonymizationAttribute.Timestamp = generationTime
	} else {
		zap.S().Error("Unable to get enough random!")
	}
}

// Returns an up to date pseudonymization attribute
func getPseudonymizationAttribute() pseudonymizationAttribute {
	timestampNow := time.Now()
	dateToday := timestampNow.YearDay()

	attributeCreationDate := globalPseudonymizationAttribute.Timestamp.YearDay()
	if globalPseudonymizationAttribute.Random == "" || dateToday != attributeCreationDate {
		// Attribute is to old
		initializeNewPseudonymizationAttribute(timestampNow)
	}
	return globalPseudonymizationAttribute
}

// Returns the most recently reported queue length, as well as the reporting unix timestamp
func GetLatestQueueLengthReport() (int, string) {
	queryString := "SELECT queueLength, MAX(time) from queueReports"

	var retrievedReportTime int
	var retrievedQueueLength string

	zap.S().Info("Querying for latest queue length report")
	db := GetDBHandle()
	if err := db.QueryRow(queryString).Scan(&retrievedQueueLength, &retrievedReportTime); err != nil {
		if err == sql.ErrNoRows {
			zap.S().Error("No rows returned when querying for latest queue length report")
		} else {
			zap.S().Error("Error while querying for latest report", err)
		}
	}

	zap.S().Infof("Queried most recent report from DB: Length %s at time %d", retrievedQueueLength, retrievedReportTime)
	return retrievedReportTime, retrievedQueueLength
}

/* getLengthsAndTimesFromRows Takes a query that contains (queueLength, times) results,
and returns them as two arrays, containing the respective data.
Times are returned in UTC.
*/
func getLengthsAndTimesFromRows(rows *sql.Rows) ([]string, []time.Time, error) {
	var err error
	var queueLengths []string
	var timesUTC []time.Time

	for rows.Next() {
		var length string
		var time time.Time
		if err = rows.Scan(&length, &time); err != nil {
			zap.S().Errorf("Error scanning for reports in weekday timeframe, likely data type mismatch", err)
		}
		queueLengths = append(queueLengths, length)
		timesUTC = append(timesUTC, time)
	}
	if err = rows.Err(); err != nil {
		zap.S().Errorf("Error while scanning for reports in timeframe", err)
		return queueLengths, timesUTC, err
	}
	zap.S().Infof("Query for reports in timeframe returned  %d reports", len(queueLengths))
	return queueLengths, timesUTC, err
}

/*
GetAllQueueLengthReportsInTimeframe returns all length reports that
were made within timeframeIntoPast before now.
Returns two slices: One with the report queue lengths,
one with the times. Returns an err if no reports are
available for that timeframe
*/
func GetAllQueueLengthReportsInTimeframe(nowUTC time.Time, timeframeIntoPast time.Duration) ([]string, []time.Time, error) {
	lowerLimit := nowUTC.Add(-timeframeIntoPast).Unix()

	queryString := "SELECT queueLength, time FROM queueReports WHERE time > ? " + // Get reports more recent than timeframe
		"AND strftime ('%s', queueReports.time, 'unixepoch') < strftime('%s', CAST(? AS TEXT)) " + // Data is not from the future, important for testing
		"ORDER BY time ASC;"

	var queueLengths []string
	var times []time.Time

	nowTimeString := nowUTC.Format("2006-01-02 15:04:05")
	zap.S().Infow("Querying for all reports in timeframe",
		"interval", timeframeIntoPast,
		"lower limit", lowerLimit,
		"nowTimeString", nowTimeString)

	db := GetDBHandle()
	rows, err := db.Query(queryString, lowerLimit, nowTimeString)
	if err != nil {
		zap.S().Errorf("Error while querying for reports in timeframe", err)
		return queueLengths, times, err
	}
	defer rows.Close()
	return getLengthsAndTimesFromRows(rows)
}

/*GetQueueLengthReportsByWeekdayAdndTimeframe returns the following reports:
- created at most daysOfDataToConsider before nowTime
- Create at most timeframeIntoPast before noTimes timestamp
- Create at most timeframeIntoFuture after noTimes timestamp
- Not created today
*/
func GetQueueLengthReportsByWeekdayAndTimeframe(daysOfDataToConsider int8,
	nowTimeUTC time.Time,
	timeframeIntoThePast time.Duration,
	timeframeIntoTheFuture time.Duration) ([]string, []time.Time, error) {

	// See https://www.sqlite.org/lang_datefunc.html for reference
	queryString := "SELECT queueLength, time from queueReports " + // Return the usual tuple
		"WHERE strftime('%s',  ? , 'unixepoch') - strftime('%s',queueReports.time, 'unixepoch', CAST(? AS TEXT)) < 0 " + // If it was created within the last 30 days
		"AND CAST(? AS TEXT) = strftime('%w', queueReports.time, 'unixepoch') " + // On the given weekday
		"AND time(queueReports.time, 'unixepoch') > CAST(? AS TEXT) " + // Start of times we're interested in
		"AND time(queueReports.time, 'unixepoch') < CAST(? AS TEXT) " + // End of times we're interested in
		"AND date(queueReports.time, 'unixepoch') != CAST(? AS TEXT) " + // Data is not from today
		"AND strftime ('%s', queueReports.time, 'unixepoch') < strftime('%s', ?, 'unixepoch');" // Data is not from the future, important for testing
	/*
		queryString := "SELECT queueLength, time from queueReports WHERE strftime('%s', '2022-11-02T16:45:00') - strftime('%s',queueReports.time, 'unixepoch', '030 days') < 0 AND '2' = strftime('%w', queueReports.time, 'unixepoch') AND time(queueReports.time, 'unixepoch', ) > '14:45:00' AND time(queueReports.time, 'unixepoch', ) < '16:15:00' AND date(queueReports.time, 'unixepoch') != '2022-11-02';"
	*/

	//Sqlite expects days we add in first strftime to be in NNN format, so let's add leading 0
	weekday := nowTimeUTC.Weekday()
	timeFrameInDaysString := fmt.Sprintf("%03d days", daysOfDataToConsider)

	nowTimestamp := nowTimeUTC.Unix()
	lowerTimeLimitString := nowTimeUTC.Add(-timeframeIntoThePast).Format("15:04:05")
	upperTimeLimitString := nowTimeUTC.Add(timeframeIntoTheFuture).Format("15:04:05")
	nowDateUTCString := nowTimeUTC.Format("2006-01-02")

	zap.S().Infow("Querying for weekdays reports in timeframe",
		"nowTimestamp", nowTimestamp,
		"timeFrameInDaysString", timeFrameInDaysString,
		"weekday", int(weekday),
		"lowerTimeLimitString", lowerTimeLimitString,
		"upperTimeLimitString", upperTimeLimitString,
		"nowDateUTCString", nowDateUTCString,
		"nowTimestamp", nowTimestamp,
	)
	var queueLengths []string
	var times []time.Time

	db := GetDBHandle()
	rows, err := db.Query(queryString, nowTimestamp, timeFrameInDaysString, int(weekday), lowerTimeLimitString, upperTimeLimitString, nowDateUTCString, nowTimestamp)

	if err != nil {
		zap.S().Errorf("Error while querying for reports in timeframe", err)
		return queueLengths, times, err
	}
	defer rows.Close()
	return getLengthsAndTimesFromRows(rows)
}

func WriteReportToDB(reporter string, time int, queueLength string) error {
	anonymizedReporter := pseudonymizeReporter(reporter)

	db := GetDBHandle()

	zap.S().Info("Writing new report into DB")
	DBMutex.Lock()
	// Nice try
	_, err := db.Exec("INSERT INTO queueReports VALUES(NULL,?,?,?);", anonymizedReporter, time, queueLength)
	DBMutex.Unlock()
	return err
}

// Returns a pseudonym for the given reporter. The pseudonym is transient, and contained within one day.
func pseudonymizeReporter(reporter string) string {
	/* We don't want to be able to track users across days, but we do want to be able to find out whether one user started spamming potentially wrong queue lengths.
	The idea is to pseudonymize with attribute day - meaning that within one day a user keeps the same pseudonym, but gets a different (not easily linkable) pseudonym on the next day.

	Specifically, we generate some Random on each day, and hash the user id given to us by telegram and that Random. Within a single day the same Random is used, and the same hash is generated. This allows for correlation within one day.
	   Across multiple days different Randoms are used, and therefore different hashes that can't be correlated are generated.

	   Randoms are discarded once a day is over, so correlation across days shouldn't be (easily) possible.

	   This scheme expects that Randoms aren't stored, or otherwise extracted. Users also can't be correlated across server restarts.
	   This scheme also expects enough randomness to be available. Handling lack of random isn't graceful - we just keep reusing the existing Random longer than we should
	   Additionally, there's the need to trust whoever operates the infrastructure, since there is no assurance towards clients that this scheme is actually used.
	*/

	randomToUse := getPseudonymizationAttribute().Random

	// We're not using bcrypt because https://pkg.go.dev/golang.org/x/crypto/bcrypt adds an additional pepper, and we want to allow for eyeball comparison of the stored values
	// for easier queue length analysis.
	hashedReporter := sha256.Sum256([]byte(reporter + randomToUse))
	return string(hashedReporter[:])
}

// Returns some save random, with the amount specified by n
// Taken from http://blog.questionable.services/article/generating-secure-random-numbers-crypto-rand/
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err == nil only if we read len(b) bytes.
	if err != nil {
		return nil, err
	}
	return b, nil
}

// GenerateRandomString returns a URL-safe, base64 encoded
// securely generated random string.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
// Taken from http://blog.questionable.services/article/generating-secure-random-numbers-crypto-rand/
func GenerateRandomString(s int) (string, error) {
	b, err := GenerateRandomBytes(s)
	return base64.URLEncoding.EncodeToString(b), err
}

func InitNewDB() error {
	const tableCreationString string = `
  CREATE TABLE IF NOT EXISTS queueReports (
  id INTEGER NOT NULL PRIMARY KEY,
  reporter TEXT NOT NULL,
  time DATETIME NOT NULL,
  queueLength TEXT NOT NULL
  );`

	db := GetDBHandle()

	zap.S().Info("Recreating database for queue length tracking...")
	if _, err := db.Exec(tableCreationString); err != nil {
		zap.S().Panic("Couldn't create report table")
		return err
	}
	return nil
}
