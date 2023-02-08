package main

/*
   - Due to the expected low utility of introducing hashes, but the real associated cost, we decide against it, and store plain user IDs instead. Since we need to send push messages for the mensa menu functionality I believe this is justified.
*/

import (
	"database/sql"
	"encoding/csv"
	"errors"
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
func GetChangelogByNumber(changelogNumber int) (string, error) {
	zap.S().Info("Getting the changelog by number")

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

	if len(changelogs) > changelogNumber {
		return changelogs[changelogNumber].Text, nil
	}
	return "", errors.New("Requested changelog is out of bounds")

}

/*
Changelogs start at ID 0, and increment one by one, per line.
*/
func GetLatestChangelogSentToUser(userID int) int {
	zap.S().Info("Querying for latest changelog for a user") // Don't log which user, that allows correlation with reports
	queryString := "SELECT lastChangelog FROM changelogMessages WHERE reporterID = ?"
	db := GetDBHandle()
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
	// Use UPSERT syntax as defined by https://www.sqlite.org/draft/lang_UPSERT.html
	queryString := "INSERT INTO changelogMessages VALUES (NULL, ?,?, 0) ON CONFLICT (reporterID) DO UPDATE SET lastChangelog=?;"
	db := GetDBHandle()

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
	queryString := "DELETE FROM changelogMessages WHERE reporterID = ?;"
	db := GetDBHandle()
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
	queryString := "SELECT ab_tester FROM changelogMessages WHERE reporterID = ?"
	db := GetDBHandle()
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
	// This assumes that all users that opt into/out of A/B tests already have a profile
	// But fails gracefully, and just does nothing except return the error if
	// that's not the case
	queryString := "UPDATE changelogMessages SET ab_tester = 1 WHERE reporterID = ?"
	db := GetDBHandle()

	DBMutex.Lock()
	_, err := db.Exec(queryString, userID)
	DBMutex.Unlock()

	if err != nil {
		zap.S().Errorf("Error while changing A/B tester status of user %s", userID, err)
		return err
	}
	return nil
}
