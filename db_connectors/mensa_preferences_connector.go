package db_connectors

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/ADimeo/MensaQueueBot/utils"
	"go.uber.org/zap"
)

// Customise:
// User has userID, times(start) (minutes), time(end) (minutes), Weekdays (binary and), wants_mensa_messages, temp_reported_today (date?)

type MensaPreferenceSettings struct {
	ReportAtAll   bool   `json:"reportAtall"`
	FromTime      string `json:"fromTime"`
	WeekdayBitmap int    `json:"weekdayBitmap"`
	ToTime        string `json:"toTime"`
}

func (settingsStruct *MensaPreferenceSettings) SetFromTimeFromCESTMinutes(cestMinutes int) {
	baseString := "%02d:%02d"
	hours := cestMinutes / 60
	minutes := cestMinutes % 60
	timeString := fmt.Sprintf(baseString, hours, minutes)
	settingsStruct.FromTime = timeString
}

func (settingsStruct *MensaPreferenceSettings) SetToTimeFromCESTMinutes(cestMinutes int) {
	baseString := "%02d:%02d"
	hours := cestMinutes / 60
	minutes := cestMinutes % 60
	timeString := fmt.Sprintf(baseString, hours, minutes)
	settingsStruct.ToTime = timeString
}

func (settingsStruct *MensaPreferenceSettings) GetToTimeAsCESTMinute() (int, error) {
	// We expect a format like 12:00
	hour, err := strconv.Atoi(settingsStruct.ToTime[0:2])
	if err != nil {
		zap.S().Errorw("Can't convert ToTime string to actual int", "FromTime value", settingsStruct.ToTime, "error", err)
		return 840, err
	}

	minute, err := strconv.Atoi(settingsStruct.ToTime[3:5])
	if err != nil {
		zap.S().Errorw("Can't convert ToTime string to actual int", "FromTime value", settingsStruct.ToTime, "error", err)
		return 840, err
	}

	cestMinute := hour*60 + minute
	if err != nil {
		zap.S().Errorw("Can't convert ToTime string to actual int", "FromTime value", settingsStruct.ToTime, "error", err)
		return 840, err
	}
	return cestMinute, nil
}

func (settingsStruct MensaPreferenceSettings) GetFromTimeAsCESTMinute() (int, error) {
	// We expect a format like 12:00
	hour, err := strconv.Atoi(settingsStruct.FromTime[0:2])
	if err != nil {
		zap.S().Errorw("Can't convert FromTime string to actual int", "FromTime value", settingsStruct.FromTime, "error", err)
		return 600, err
	}
	minute, err := strconv.Atoi(settingsStruct.FromTime[3:5])
	if err != nil {
		zap.S().Errorw("Can't convert FromTime string to actual int", "FromTime value", settingsStruct.FromTime, "error", err)
		return 600, err
	}

	cestMinute := hour*60 + minute
	if err != nil {
		zap.S().Errorw("Can't convert FromTime string to actual int", "FromTime value", settingsStruct.FromTime, "error", err)
		return 600, err
	}
	return cestMinute, nil
}

func GetUsersToSendMenuToByTimestamp(nowInUTC time.Time) ([]int, error) {
	queryString := `SELECT reporterID FROM mensaPreferences
		WHERE wantsMensaMessages = 1
		AND (lastReportDate IS NULL OR date(lastReportDate) != ?)
		AND ? BETWEEN startTimeInCESTMinutes AND endTimeInCESTMinutes
		AND ? & weekdayBitmap > 0;`

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

/*
returns the CEST Minute that would next run after the given cestMinuteOfLastRun,
based on some extra conditions (weekday, already reported today, wants messages)
a cest Minute is an int with hh*60 + mm of a timestamp.
Uses nowInUTC to find out the date, and cestMinuteOfLastRun for the time
*/
func GetCESTMinuteForNextIntroMessage(nowInUTC time.Time, cestMinuteOfLastRun int) (int, error) {
	queryString := `SELECT startTimeInCESTMinutes FROM mensaPreferences 
	WHERE startTimeInCESTMinutes > ? 
	AND wantsMensaMessages = 1
	AND (lastReportDate IS NULL OR date(lastReportDate) != ?)
	AND ? & weekdayBitmap > 0
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
	queryString := `SELECT IFNULL(MIN(startTimeInCESTMinutes), 0) FROM mensaPreferences
	WHERE wantsMensaMessages = 1;`
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
		AND ? & weekdayBitmap > 0;`

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
	if len(userIDs) == 0 {
		zap.S().Infow("Query for users with initial message returned empty!",
			"currentDate", currentDate,
			"lowerBoundCESTMinute", lowerBoundCESTMinute,
			"upperBoundCESTMinute", upperBoundCESTMinute,
			"weekdayBitmap", weekdayBitmap)
	}
	return userIDs, nil
}

func UpdateUserPreferences(userID int, wantsMensaMessages bool, startTimeInCESTMinutes int, endTimeInCESTMinutes int, weekdayBitmap int) error {
	queryString := "INSERT INTO mensaPreferences(reporterID, wantsMensaMessages, startTimeInCESTMinutes, endTimeInCESTMinutes, weekdayBitmap) VALUES (?,?,?,?,?) ON CONFLICT (reporterID) DO UPDATE SET wantsMensaMessages=?, startTimeInCESTMinutes=?, endTimeInCESTMinutes=?,weekdayBitmap=?;"
	db := GetDBHandle()
	DBMutex.Lock()

	_, err := db.Exec(queryString, userID, wantsMensaMessages, startTimeInCESTMinutes, endTimeInCESTMinutes, weekdayBitmap,
		wantsMensaMessages, startTimeInCESTMinutes, endTimeInCESTMinutes, weekdayBitmap)
	DBMutex.Unlock()
	return err
}

// Returns preferences for single user. Sets preferences to default
// And returns those if user doesn't have any associated preferences
func GetUserPreferences(userID int) (MensaPreferenceSettings, error) {
	queryString := `SELECT wantsMensaMessages, weekdayBitmap, startTimeInCESTMinutes, endTimeInCESTMinutes 
	FROM mensaPreferences
	WHERE reporterID == ?;`

	db := GetDBHandle()
	var usersPreferences MensaPreferenceSettings
	var weekdayBitmap int
	var startCESTMinutes int
	var endCESTMinutes int

	if err := db.QueryRow(queryString, userID).Scan(&usersPreferences.ReportAtAll, &weekdayBitmap, &startCESTMinutes, &endCESTMinutes); err != nil {
		if err == sql.ErrNoRows {
			zap.S().Info("User doesn't have associated mensa settings yet")
			usersPreferences, err = SetDefaultMensaPreferencesForUser(userID)
			if err != nil {
				return usersPreferences, err
			}
		} else {
			zap.S().Error("Error while querying for latest report", err)
		}
	}

	usersPreferences.WeekdayBitmap = weekdayBitmap
	usersPreferences.SetFromTimeFromCESTMinutes(startCESTMinutes)
	usersPreferences.SetToTimeFromCESTMinutes(endCESTMinutes)

	return usersPreferences, nil
}
func SetDefaultMensaPreferencesForUser(userID int) (MensaPreferenceSettings, error) {
	var err error
	var usersPreferences MensaPreferenceSettings
	usersPreferences.ReportAtAll = true
	if utils.IsInDebugMode() {
		err = UpdateUserPreferences(userID, true, 0, 1440, 0b0111110) // Default from 0:00 to 24:00
		usersPreferences.WeekdayBitmap = 0b0111110
		usersPreferences.SetFromTimeFromCESTMinutes(0)
		usersPreferences.SetToTimeFromCESTMinutes(1440)

	} else {
		err = UpdateUserPreferences(userID, true, 600, 840, 0b0111110) // Default from 10:00 to 14:00
		usersPreferences.WeekdayBitmap = 0b0111110
		usersPreferences.SetFromTimeFromCESTMinutes(600)
		usersPreferences.SetToTimeFromCESTMinutes(840)
	}

	if err != nil {
		zap.S().Errorf("Can't set default preferences for user %d", userID, err)
	}
	return usersPreferences, err
}

func DeleteAllUserMensaPreferences(userID int) error {
	queryString := `DELETE FROM mensaPreferences WHERE reporterID == ?;`
	db := GetDBHandle()
	zap.S().Infof("Deleting mensa preferences for user %d", userID)

	DBMutex.Lock()
	_, err := db.Exec(queryString, userID)
	DBMutex.Unlock()
	if err != nil {
		zap.S().Errorf("Error while deleting mensa preferences of user %s", userID, err)
		return err
	}
	return nil
}

func SetUserToReportedOnDate(userID int, nowInUTC time.Time) error {
	queryString := `UPDATE mensaPreferences
		SET lastReportDate = ?
		WHERE reporterID = ?;`
	db := GetDBHandle()

	currentDate := nowInUTC.Format("2006-01-02")

	DBMutex.Lock()
	_, err := db.Exec(queryString, currentDate, userID)
	DBMutex.Unlock()
	if err != nil {
		zap.S().Errorw("Error while saving users report date %s",
			"userID", userID,
			"currentDate", currentDate,
			"err", err)
		return err
	}
	return nil
}

func getBitmapForToday(nowInUTC time.Time) int {
	weekdayNow := nowInUTC.Weekday() // Sunday is 0, Sunday is left, shift 6 for sunday
	weekdayBitmap := 1 << (6 - weekdayNow)
	return weekdayBitmap
}
