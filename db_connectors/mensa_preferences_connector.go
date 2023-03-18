package db_connectors

import (
	"database/sql"
	"time"

	"github.com/ADimeo/MensaQueueBot/utils"
	"go.uber.org/zap"
)

// Customise:
// User has userID, times(start) (minutes), time(end) (minutes), Weekdays (binary and), wants_mensa_messages, temp_reported_today (date?)

func GetUsersToSendMenuToByTimestamp(nowInUTC time.Time) ([]int, error) {
	queryString := `SELECT reporterID FROM mensaPreferences
		WHERE wantsMensaMessages = 1
		AND (lastReportDate IS NULL OR date(lastReportDate) != ?)
		AND ? BETWEEN startTimeInCESTMinutes AND endTimeInCESTMinutes
		AND ? & weekdayBitmap >= 0;`

	currentDate := nowInUTC.Format("2006-01-02")
	currentCESTDate := nowInUTC.In(utils.GetLocalLocation())
	currentCESTMinute := currentCESTDate.Hour()*60 + currentCESTDate.Minute()

	weekdayBitmap := getBitmapForToday(nowInUTC)

	db := GetDBHandle()
	rows, err := db.Query(queryString, currentDate, currentCESTMinute, weekdayBitmap)
	if err != nil {
		zap.S().Errorf("Couldn't get users to send menu to", err)
		return make([]int, 0), err
	}
	defer rows.Close()

	var userIDs []int
	for rows.Next() {
		var userID int
		if err = rows.Scan(&userID); err != nil {
			zap.S().Errorf("Couldn't put user ID into int, likely data type mismatch", err)
		}
		userIDs = append(userIDs, userID)
	}
	if err = rows.Err(); err != nil {
		zap.S().Errorf("Error while scanning userids for mensa preferences", err)
		return userIDs, err
	}
	return userIDs, nil
}

func GetCESTMinuteForNextIntroMessage(nowInUTC time.Time, cestMinuteOfLastRun int) (int, error) {
	queryString := `SELECT startTimeInCESTMinutes FROM mensaPreferences 
	WHERE startTimeInCESTMinutes > ? 
	AND wantsMensaMessages = 1
	AND (lastReportDate IS NULL OR date(lastReportDate) != ?)
	AND ? & weekdayBitmap >= 0
	ORDER BY startTimeInCESTMinutes ASC 
	LIMIT 1;`

	currentDate := nowInUTC.Format("2006-01-02")
	weekdayBitmap := getBitmapForToday(nowInUTC)

	db := GetDBHandle()
	var nextCESTMinute int

	if err := db.QueryRow(queryString, cestMinuteOfLastRun, currentDate, weekdayBitmap).Scan(&nextCESTMinute); err != nil {
		if err == sql.ErrNoRows {
			zap.S().Debug("No more startTimes scheduled for today")
			return GetFirstCESTMinuteForIntroMessage()
		} else {
			zap.S().Errorw("Error while querying for next mensa report", err)
			// We default to scheduling the first "welcome" job at 08:00
			return 8 * 60, nil
		}
	}
	return nextCESTMinute, nil
}

func GetFirstCESTMinuteForIntroMessage() (int, error) {
	// We ignore the weekday in this query.
	// _technically_ this is a bug, because it will schedule the job
	// on hours when there's nothing to run (because the user that would have the
	// initial message doesn't want it this weekday)
	// But, well, it's invisible to users,
	// And getting this behaviour cleanly (so wrapping arround the weekday bitmap,
	// etc.) just doesn't feel worth it at all.
	queryString := `SELECT MIN(startTimeInCESTMinutes) FROM mensaPreferences
	WHERE wantsMensaMessage = 1
	FROM mensaPreferences;`
	db := GetDBHandle()

	var firstTime int
	if err := db.QueryRow(queryString).Scan(&firstTime); err != nil {
		if err == sql.ErrNoRows {
			zap.S().Info("Can't find a single mensa report start time")
			// We default to scheduling the first "welcome" job at 08:00
			return 8 * 60, nil
		} else {
			zap.S().Errorw("Error while querying for first mensa report", err)
			// We default to scheduling the first "welcome" job at 08:00
			return 8 * 60, nil
		}
	}
	return firstTime, nil
}

func GetUsersWithInitialMessageInTimeframe(nowInUTC time.Time, lowerBoundCESTMinute int, upperBoundCESTMinute int) ([]int, error) {
	// +1 in BETWEEN because we don't want to include the lower bound in the interval,
	// and SQLites BETWEEN statement is inclusive of both upper and lower bound
	queryString := `SELECT reporterID FROM mensaPreferences
		WHERE wantsMensaMessages = 1
		AND (lastReportDate IS NULL OR date(lastReportDate) != ?)
		AND startTimeInCESTMinutes BETWEEN ? + 1 AND ? 
		AND ? & weekdayBitmap >= 0;`

	currentDate := nowInUTC.Format("2006-01-02")
	weekdayBitmap := getBitmapForToday(nowInUTC)

	db := GetDBHandle()
	rows, err := db.Query(queryString, currentDate, lowerBoundCESTMinute, upperBoundCESTMinute, weekdayBitmap)
	if err != nil {
		zap.S().Errorf("Couldn't get users to send menu to", err)
		return make([]int, 0), err
	}
	defer rows.Close()

	var userIDs []int
	for rows.Next() {
		var userID int
		if err = rows.Scan(&userID); err != nil {
			zap.S().Errorf("Couldn't put user ID into int, likely data type mismatch", err)
		}
		userIDs = append(userIDs, userID)
	}
	if err = rows.Err(); err != nil {
		zap.S().Errorf("Error while scanning userids for mensa preferences", err)
		return userIDs, err
	}
	return userIDs, nil
}

func UpdateUserPreferences(userID int, wantsMensaMessages bool, startTimeInCESTMinutes int, endTimeInCESTMinutes int, weekdayBitmap int) error {
	queryString := "INSERT INTO mensaPreferences(reporterID, wantsMensaMessages, startTimeInCESTinutes, endTimeInCESTMinutes, weekdayBitmap) VALUES (?,?,?,?,?) ON CONFLICT (reporterID) DO UPDATE SET wantsMensaMessages=?, startTimeInCESTMinutes=?, endTimeInCESTMinutes=?,weekdayBitmap=?;"
	db := GetDBHandle()
	DBMutex.Lock()

	_, err := db.Exec(queryString, userID, wantsMensaMessages, startTimeInCESTMinutes, endTimeInCESTMinutes, weekdayBitmap,
		wantsMensaMessages, startTimeInCESTMinutes, endTimeInCESTMinutes, weekdayBitmap)
	DBMutex.Unlock()
	return err
}

func SetUserToReportedOnDate(userID int, nowInUTC time.Time) {
	/*
		queryString := `UPDATE mensaPreferences
		SET lastReportDate = ?
		WHERE reporterID = ?;`

		db := GetDBHandle()
		// TODO finish query
		**/

}

func getBitmapForToday(nowInUTC time.Time) int {
	weekdayNow := nowInUTC.Weekday() // Sunday is 0, Sunday is left, shift 6 for sunday
	weekdayBitmap := 1 << (6 - weekdayNow)
	return weekdayBitmap
}
