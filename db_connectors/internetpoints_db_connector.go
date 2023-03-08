package db_connectors

import (
	"database/sql"

	"go.uber.org/zap"
)

func UserIsCollectingPoints(userID int) bool {
	if GetNumberOfPointsByUser(userID) == -1 {
		return false
	} else {
		return true
	}
}

func GetNumberOfPointsByUser(userID int) int {
	queryString := "SELECT points FROM internetpoints WHERE reporterID= ?;"
	db := GetDBHandle()
	var numberOfPoints int

	zap.S().Infof("Querying for points of user %d", userID)

	if err := db.QueryRow(queryString, userID).Scan(&numberOfPoints); err != nil {
		if err == sql.ErrNoRows {
			zap.S().Info("No points returned, returning -1 ")
			numberOfPoints = -1
		} else {
			zap.S().Errorw("Error while querying for points", err)
			numberOfPoints = -1
		}
	}
	return numberOfPoints
}

func AddInternetPoint(userID int) error {
	queryString := "UPDATE internetpoints SET points = points + 1 WHERE reporterID= ?;"
	db := GetDBHandle()

	zap.S().Info("Adding point to user") // Don't log user explicitly for anonymity

	DBMutex.Lock()
	_, err := db.Exec(queryString, userID)
	DBMutex.Unlock()
	if err != nil {
		zap.S().Errorf("Error adding new internetpoint for user %s", userID, err)
		return err
	}
	return nil
}

func EnableCollectionOfPoints(userID int) error {
	queryString := "INSERT INTO internetpoints VALUES (NULL, ?, 0);"
	db := GetDBHandle()

	zap.S().Infof("Enabling point collection for user %d", userID)

	DBMutex.Lock()
	_, err := db.Exec(queryString, userID)
	DBMutex.Unlock()
	if err != nil {
		zap.S().Errorf("Error while enabling internetpoints for user %s", userID, err)
		return err
	}
	return nil
}

func DisableCollectionOfPoints(userID int) error {
	// Note: If this is changed from deleting all user data also modify DeleteAllUserPointData for compliance
	queryString := "DELETE FROM internetpoints WHERE reporterID = ?;"
	db := GetDBHandle()

	zap.S().Infof("Disabling point collection for user %d", userID)

	DBMutex.Lock()
	_, err := db.Exec(queryString, userID)
	DBMutex.Unlock()
	if err != nil {
		zap.S().Errorf("Error while deleting internetpoints of user %s", userID, err)
		return err
	}
	return nil
}

func DeleteAllUserPointData(userID int) error {
	// Deleting user data is exactly what happens when we call DisableCollectionOfPoints
	return DisableCollectionOfPoints(userID)
}
