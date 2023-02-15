package main

import (
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"go.uber.org/zap"
)

const TEST_DB_PATH string = "./for_tests.db"

func initializeForTest() {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	initializeTestDB()
}

// Creates a new empty DB
func initializeTestDB() {
	db_handle := GetTestDBHandle(TEST_DB_PATH)
	// Let's just assume the migrations work...
	driver, _ := sqlite3.WithInstance(db_handle, &sqlite3.Config{})
	m, _ := migrate.NewWithDatabaseInstance("file://./db/migrations", "sqlite3", driver)
	m.Migrate(DB_VERSION) // Variable from db_utilities
	// Initialization done
}

func resetTestDB() {
	// Remove the DB file
	err := os.Remove(TEST_DB_PATH)
	if err != nil {
		zap.S().Panic("Can't delete test DB after test!")
	}
}

// Tests for the functions within changelog_db_connector
func TestWriteAndReadOfChangelogs(t *testing.T) {
	initializeForTest()
	defer resetTestDB()
	userID := 12345
	changelogID := 123

	db := GetTestDBHandle(TEST_DB_PATH)

	retrievedChangelogID := getLatestChangelogSentToUserWithDB(userID, db)
	if retrievedChangelogID != -1 {
		// Default to -1 for users that don't exist
		t.Fail()
	}

	err := saveNewChangelogForUserWithDB(userID, changelogID, db)
	if err != nil {
		t.Fail()
	}

	retrievedChangelogID = getLatestChangelogSentToUserWithDB(userID, db)
	if retrievedChangelogID != changelogID {
		t.Fail()
	}
}

func TestDeletionOfUserChangelog(t *testing.T) {
	initializeForTest()
	defer resetTestDB()
	userID := 12346
	changelogID := 126

	db := GetTestDBHandle(TEST_DB_PATH)
	saveNewChangelogForUserWithDB(userID, changelogID, db)

	retrievedChangelogID := getLatestChangelogSentToUserWithDB(userID, db)
	if retrievedChangelogID == -1 {
		t.Fail()
	}

	deleteAllUserChangelogDataWithDB(userID, db)

	retrievedChangelogID = getLatestChangelogSentToUserWithDB(userID, db)
	if retrievedChangelogID != -1 {
		t.Fail()
	}

}

func TestChangingABTesterState(t *testing.T) {
	initializeForTest()
	defer resetTestDB()
	userID := 12347
	changelogID := 127

	db := GetTestDBHandle(TEST_DB_PATH)

	saveNewChangelogForUserWithDB(userID, changelogID, db)
	isABTester := getIsUserABTesterWithDB(userID, db)
	if isABTester {
		t.Errorf("Users don't default to not being AB testers")
	}

	makeUserABTesterWithDB(userID, true, db)
	isABTester = getIsUserABTesterWithDB(userID, db)
	if !isABTester {
		t.Errorf("Can't make users AB testers")
	}

	makeUserABTesterWithDB(userID, false, db)
	isABTester = getIsUserABTesterWithDB(userID, db)
	if isABTester {
		t.Errorf("Can't unmake users AB testers")
	}
}
