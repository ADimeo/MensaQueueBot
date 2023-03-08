/*
Utility functions that are useful to all entities that interact with the DB.
Notaby, implements the DB handle
*/
package db_connectors

import (
	"database/sql"
	"os"
	"sync"

	"go.uber.org/zap"
)

const KEY_DB_BASE_PATH string = "MENSA_QUEUE_BOT_DB_PATH"
const DB_NAME string = "queue_database.db"
const DB_VERSION uint = 4

var globalDBHandle *sql.DB = nil

// Needs to be used by all outside functions that request a DB handle
var DBMutex sync.Mutex

func GetDBHandle() *sql.DB {
	dbPath, doesExist := os.LookupEnv(KEY_DB_BASE_PATH)
	dbPath = dbPath + DB_NAME

	if !doesExist {
		zap.S().Panic("Fatal Error: Environment variable for personal key not set:", KEY_DB_BASE_PATH)
	}

	if globalDBHandle == nil {
		// init db
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			zap.S().Panicf("Couldn't get DB handle with path %s", dbPath)

		}
		globalDBHandle = db
	}
	return globalDBHandle
}

// A separate DB, used only for testing
func GetTestDBHandle(dbPath string) *sql.DB {
	// Unlike with the actual DB, don't use a global handle.
	// Might want to run tests on different DBs, and we do not
	// want to touch global state in tests
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		zap.S().Panicf("Couldn't get DB handle with path %s", dbPath)
	}
	return db
}

func GetDBVersion() uint {
	return DB_VERSION
}
