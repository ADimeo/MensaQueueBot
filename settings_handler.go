package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"go.uber.org/zap"
)

func HandleAccountDeletion(chatID int) {
	err1 := db_connectors.DeleteAllUserPointData(chatID)
	err2 := db_connectors.DeleteAllUserChangelogData(chatID)
	if err1 != nil || err2 != nil {
		zap.S().Infof("Sending error message to user")
		keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.SETTINGS_INTERACTION, chatID)
		telegram_connector.SendMessage(chatID, "Something went wrong deleting your data. Contact @adimeo for details and fixes", keyboardIdentifier)
		zap.S().Warn("Error in forgetme: ", err1, err2)
	} else {
		keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.ACCOUNT_DELETION, chatID)
		telegram_connector.SendMessage(chatID, "Who are you again? I have completely forgotten you exist. Remind me with /start, please?", keyboardIdentifier)
	}
}

func HandleABTestJoining(chatID int) {
	err := db_connectors.MakeUserABTester(chatID, true)
	ABTestHandler(chatID)
	keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.PREPARE_MAIN, chatID)
	if err != nil {
		telegram_connector.SendMessage(chatID, "Something went wrong, please try again later ", keyboardIdentifier)
		zap.S().Warn("Error in A/B opt in: ", err)
	} else {
		telegram_connector.SendMessage(chatID, "Welcome to the test crew 🫡", keyboardIdentifier)
	}
}

func HandleSettingsChange(chatID int, webAppData telegram_connector.WebhookRequestBodyWebAppData) {
	typeOfKeyboard := webAppData.ButtonText
	if typeOfKeyboard == "Change Settings" {
		jsonString := webAppData.Data
		var mensaSettings db_connectors.MensaPreferenceSettings
		err := json.Unmarshal([]byte(jsonString), &mensaSettings)
		if err != nil {
			zap.S().Errorw("Can't unmarshal the settings json we got as WebAppData", "json", jsonString, "error", err)
		}

		startCESTMinutes, _ := mensaSettings.GetFromTimeAsCESTMinute() // These functions default to acceptable values, even on errors
		endCESTMinutes, _ := mensaSettings.GetToTimeAsCESTMinute()

		if err := db_connectors.UpdateUserPreferences(chatID, mensaSettings.ReportAtAll, startCESTMinutes, endCESTMinutes, mensaSettings.WeekdayBitmap); err != nil {
			zap.S().Warnw("Can't update user preferences", "chatID", chatID, err)
			message := "Error saving settings, please try again later"
			keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.SETTINGS_INTERACTION, chatID)
			telegram_connector.SendMessage(chatID, message, keyboardIdentifier)
		} else {
			message := "Successfully saved your settings"
			keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.SETTINGS_INTERACTION, chatID)
			telegram_connector.SendMessage(chatID, message, keyboardIdentifier)
		}
	} else {
		zap.S().Errorw("Unknown button used to send webhook to us", "Button title", typeOfKeyboard, "Data", webAppData.Data)

	}

}

func SendSettingsOverviewMessage(chatID int) error {
	baseMessage := `<b>Settings</b>`
	var lengthReportMessage string
	var pointsReportMessage string
	var abTesterMessage string

	userPreferences, err := db_connectors.GetUserPreferences(chatID)
	if err != nil {
		// TODO
	}
	lengthReportMessage = buildLengthReportMessage(userPreferences)
	pointsReportMessage = buildPointsReportMessage(chatID)

	message := baseMessage + "\n\n" + lengthReportMessage + "\n" + pointsReportMessage

	if db_connectors.GetIsUserABTester(chatID) {
		abTesterMessage = buildABTesterMessage(chatID)
		message = message + "\n" + abTesterMessage
	}

	keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.PREPARE_SETTINGS, chatID)
	return telegram_connector.SendMessage(chatID, message, keyboardIdentifier)
}

func buildLengthReportMessage(userPreferences db_connectors.MensaPreferenceSettings) string {
	noMessage := "You are not receiving any mensa menus."
	yesMessage := "You are receiving menus on %s, from %s to %s"

	if !userPreferences.ReportAtAll || userPreferences.WeekdayBitmap == 0 {
		return noMessage
	} else {
		weekdaysString := ""
		if userPreferences.WeekdayBitmap&0b0100000 != 0 {
			weekdaysString += ", Mondays"
		}
		if userPreferences.WeekdayBitmap&0b0010000 != 0 {
			weekdaysString += ", Tuesdays"
		}
		if userPreferences.WeekdayBitmap&0b0001000 != 0 {
			weekdaysString += ", Wednesdays"
		}
		if userPreferences.WeekdayBitmap&0b0000100 != 0 {
			weekdaysString += ", Thursdays"
		}
		if userPreferences.WeekdayBitmap&0b0000010 != 0 {
			weekdaysString += ", Fridays"
		}
		weekdaysString = weekdaysString[2:]
		lastCommaPosition := strings.LastIndex(weekdaysString, ",") + 1
		weekdaysString = weekdaysString[:lastCommaPosition] + " and" + weekdaysString[lastCommaPosition:]
		return fmt.Sprintf(yesMessage, weekdaysString, userPreferences.FromTime, userPreferences.ToTime)
	}
}

func buildPointsReportMessage(chatID int) string {
	pointsMessage := GetPointsRequestResponseText(chatID)
	return pointsMessage
}

func buildABTesterMessage(chatID int) string {
	if db_connectors.GetIsUserABTester(chatID) {
		return "You are currently opted in to test new features"
	} else {
		return "You are currently not a beta tester"
	}
}
