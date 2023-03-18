package main

import (
	"encoding/json"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/mensa_scraper"
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
		telegram_connector.SendMessage(chatID, "Who are you again? I have completely forgotten you exist.", keyboardIdentifier)
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
	if typeOfKeyboard == "TEST" {
		jsonString := webAppData.Data
		var mensaSettings mensa_scraper.MensaPreferenceSettings
		err := json.Unmarshal([]byte(jsonString), &mensaSettings)
		if err != nil {
			zap.S().Errorw("Can't unmarshal the settings json we got as WebAppData", "json", jsonString, "error", err)
		}

		weekdayBitmap := mensaSettings.GetWeekdayBitmap()
		startCESTMinutes, _ := mensaSettings.GetFromTimeAsCESTMinute() // These functions default to acceptable values, even on errors
		endCESTMinutes, _ := mensaSettings.GetToTimeAsCESTMinute()

		if err := db_connectors.UpdateUserPreferences(chatID, mensaSettings.ReportAtAll, startCESTMinutes, endCESTMinutes, weekdayBitmap); err != nil {
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
