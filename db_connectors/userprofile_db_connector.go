package db_connectors

/*
   - Due to the expected low utility of introducing hashes, but the real associated cost, we decide against it, and store plain user IDs instead. Since we need to send push messages for the mensa menu functionality I believe this is justified.
*/

import (
	"database/sql"
	"encoding/csv"
	"io"
	"os"
	"strconv"

	"go.uber.org/zap"
)

type changelog struct {
	Id   int
	Text string
}

const CHANGELOG_FILE_LOCATION = "./changelog.psv"

var changelogs []changelog

/* Assumes one changelog per line, with "version" always increasing one-by-one*/
func GetCurrentChangelog() (changelog, error) {
	zap.S().Debug("Getting latest changelog...")

	if len(changelogs) == 0 {
		// load changelogs into memory if they aren't loaded yet
		zap.S().Info("Loading changelog from disk")
		csvFile, err := os.Open(CHANGELOG_FILE_LOCATION)
		if err != nil {
			zap.S().Panicf("Can't access changelog psv file at %s", CHANGELOG_FILE_LOCATION)
		}
		defer csvFile.Close()

		psvReader := csv.NewReader(csvFile)
		psvReader.Comma = '|' // Pipe separated file

		for {
			record, err := psvReader.Read()
			if err == io.EOF {
				zap.S().Infof("Loaded %d changelogs", len(changelogs))
				break
			}
			if err != nil {
				zap.S().Panicf("Can't read psv file at %s", CHANGELOG_FILE_LOCATION)
			}
			changelogID, conversionErr := strconv.Atoi(record[0])
			if conversionErr != nil {
				zap.S().Panicf("Bad PSV entry: Can't convert changelog ID %s", record[0])
			}
			readChangelog := changelog{Id: changelogID, Text: record[1]}
			changelogs = append(changelogs, readChangelog)
		}
	}
	return changelogs[len(changelogs)], nil
}

/*
Changelogs start at ID 0, and increment one by one, per line.
Wrapper for testing.
*/
func GetLatestChangelogSentToUser(userID int) int {
	db := GetDBHandle()
	return getLatestChangelogSentToUserWithDB(userID, db)
}

func getLatestChangelogSentToUserWithDB(userID int, db *sql.DB) int {
	zap.S().Info("Querying for latest changelog for a user") // Don't log which user, that allows correlation with reports
	queryString := "SELECT lastChangelog FROM changelogMessages WHERE reporterID = ?"
	var retrievedLastChangelog int

	if err := db.QueryRow(queryString, userID).Scan(&retrievedLastChangelog); err != nil {
		if err == sql.ErrNoRows {
			zap.S().Info("No changelog returned, returning -1 ")
			retrievedLastChangelog = -1
		} else {
			zap.S().Errorw("Error while querying for changelog", err)
			retrievedLastChangelog = -1
		}
	}

	return retrievedLastChangelog
}

func SaveNewChangelogForUser(userID int, changelogID int) error {
	db := GetDBHandle()
	return saveNewChangelogForUserWithDB(userID, changelogID, db)
}
func saveNewChangelogForUserWithDB(userID int, changelogID int, db *sql.DB) error {
	// Use UPSERT syntax as defined by https://www.sqlite.org/draft/lang_UPSERT.html
	queryString := "INSERT INTO changelogMessages VALUES (NULL, ?,?, 0) ON CONFLICT (reporterID) DO UPDATE SET lastChangelog=?;"

	zap.S().Info("Inserting changelog sent into DB") // Don't log which user, that allows correlation with reports

	DBMutex.Lock()
	_, err := db.Exec(queryString, userID, changelogID, changelogID)
	DBMutex.Unlock()
	if err != nil {
		zap.S().Errorf("Error while inserting new changelog", err)
		return err
	}
	return nil
}

func DeleteAllUserChangelogData(userID int) error {
	db := GetDBHandle()
	return deleteAllUserChangelogDataWithDB(userID, db)
}

func deleteAllUserChangelogDataWithDB(userID int, db *sql.DB) error {
	queryString := "DELETE FROM changelogMessages WHERE reporterID = ?;"
	zap.S().Infof("Deleting changelog info for user %d", userID)
	DBMutex.Lock()
	_, err := db.Exec(queryString, userID)
	DBMutex.Unlock()

	if err != nil {
		zap.S().Errorf("Error while deleting changelogs of user %s", userID, err)
		return err
	}

	return nil
}

func GetIsUserABTester(userID int) bool {
	db := GetDBHandle()
	return getIsUserABTesterWithDB(userID, db)
}

func getIsUserABTesterWithDB(userID int, db *sql.DB) bool {
	queryString := "SELECT ab_tester FROM changelogMessages WHERE reporterID = ?"
	var isABTester int

	if err := db.QueryRow(queryString, userID).Scan(&isABTester); err != nil {
		if err == sql.ErrNoRows {
			zap.S().Info("No state returned, defaulting to false")
			return false
		} else {
			zap.S().Errorw("Error while querying for A/B tester state", err)
			return false
		}
	}

	return isABTester == 1
}

func MakeUserABTester(userID int, optingIn bool) error {
	db := GetDBHandle()
	return makeUserABTesterWithDB(userID, optingIn, db)
}

func makeUserABTesterWithDB(userID int, optingIn bool, db *sql.DB) error {
	// This assumes that all users that opt into/out of A/B tests already have a profile
	// But fails gracefully, and just does nothing except return the error if
	// that's not the case
	queryString := "UPDATE changelogMessages SET ab_tester = ? WHERE reporterID = ?"

	DBMutex.Lock()
	_, err := db.Exec(queryString, optingIn, userID)
	DBMutex.Unlock()

	if err != nil {
		zap.S().Errorf("Error while changing A/B tester status of user %s", userID, err)
		return err
	}
	return nil
}
