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
		telegram_connector.SendMessage(chatID, "Something went wrong deleting your data. Contact @adimeo for details and fixes")
		zap.S().Warnf("Error in forgetme: ", err1, err2)
	} else {
		telegram_connector.SendMessage(chatID, "Who are you again? I have completely forgotten you exist.")
	}
}

func HandleABTestJoining(chatID int) {
	err := db_connectors.MakeUserABTester(chatID, true)
	ABTestHandler(chatID)
	if err != nil {
		telegram_connector.SendMessage(chatID, "Something went wrong, please try again later ")
		zap.S().Warnf("Error in A/B opt in: ", err)
	} else {
		telegram_connector.SendMessage(chatID, "Welcome to the test crew 🫡")
	}
}
