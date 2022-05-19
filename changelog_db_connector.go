package main

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
	queryString := "INSERT INTO changelogMessages VALUES (NULL, ?,?) ON CONFLICT (reporterID) DO UPDATE SET lastChangelog=?;"
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

func InitNewChangelogDB() error {
	/*
	   - Due to the expected low utility of introducing hashes, but the real associated cost, we decide against it, and store plain user IDs instead.

	   - We have seen some... enthusiastic users, and hashes would need to be calculated on every message that's sent us. Hashing is a relatively expensive operation
	   - Hashing won't stop attackers from being able to identify whether a certain person is a user: IDs should be considered public
	   - But would make it harder to go the other way, to identify the users from a breach
	       - Still, the positive impact is quite limited, due to the low entropy of IDs
	       - There is no realisitc threat that would be interested in such an attack.
	   - Not hashing would allow for sending push-messages, which may not be a wanted functionality
	*/

	const tableCreationString string = `
  CREATE TABLE IF NOT EXISTS changelogMessages (
  id INTEGER NOT NULL PRIMARY KEY,
  reporterID INTEGER UNIQUE NOT NULL, 
  lastChangelog INTEGER NOT NULL
  );`

	db := GetDBHandle()

	zap.S().Info("Recreating database for changelog tracking...")
	if _, err := db.Exec(tableCreationString); err != nil {
		zap.S().Panicf("Couldn't create changelog table", err)
		return err
	}
	return nil
}
