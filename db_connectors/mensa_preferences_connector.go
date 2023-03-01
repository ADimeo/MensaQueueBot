package db_connectors

import (
	"database/sql"
	"time"

	"go.uber.org/zap"
)

// Customise:
// User has userID, times(start) (minutes), time(end) (minutes), Weekdays (binary and), wants_mensa_messages, temp_reported_today (date?)

func GetUsersToSendMenuToByTimestamp(nowInUTC time.Time) ([]int, error) {
	queryString := `SELECT reporterID FROM mensaPreferences
		WHERE wantsMensaMessages = 1
		AND date(lastReportDate) != ?
		AND ? BETWEEN startTimeInSeconds AND endTimeInSeconds
		AND ? & weekdayBitmap == 1;`

	currentDate := nowInUTC.Format("2006-01-02")
	currentTime := nowInUTC.Hour()*60 + nowInUTC.Minute()
	weekdayNow := nowInUTC.Weekday() // Sunday is 0, Sunday is left, shift 6 for sunday
	weekdayBitmap := 1 << (6 - weekdayNow)

	db := GetDBHandle()
	rows, err := db.Query(queryString, currentDate, currentTime, weekdayBitmap)
	if err != nil {
		zap.S().Errorf("Couldn't get users to send menu to", err)
		return make([]int, 0), err
	}

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

func GetTimeForNextIntroMessage(nowInUTC time.Time) (int, error) {
	queryString := `SELECT startTimeInSeconds FROM mensaPreferences 
	WHERE startTimeInSeconds > ? 
	ORDER BY startTimeInSeconds ASC 
	LIMIT 1;`

	db := GetDBHandle()
	var nextTime int

	if err := db.QueryRow(queryString, nowInUTC).Scan(&nextTime); err != nil {
		if err == sql.ErrNoRows {
			zap.S().Debug("No more startTimes scheduled for today")
			return GetFirstTimeForIntroMessage()
		} else {
			zap.S().Errorw("Error while querying for next mensa report", err)
			// We default to scheduling the first "welcome" job at 08:00
			return 8 * 60, nil
		}
	}
	return nextTime, nil
}

func GetFirstTimeForIntroMessage() (int, error) {
	queryString := `SELECT MIN(startTimeInSeconds) FROM mensaPreferences;`
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

func UpdateUserPreferences() {
	// TODO finish query

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
