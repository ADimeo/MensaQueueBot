package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"github.com/ADimeo/MensaQueueBot/utils"
	"go.uber.org/zap"
)

/*
HandleNavigationToReportKeyboard handles navigation to the report keyboard.
This includes the actual navigation (sending out a message with the new keyboard,
as well as storing the users intent in the DB, which is used to id whether they've
reported somethign on this day (for mensa updates)
*/
func HandleNavigationToReportKeyboard(sentMessage string, chatID int) {
	message := "Great! How is it looking?"
	nowInUTCTime := time.Now().UTC()
	db_connectors.SetUserToReportedOnDate(chatID, nowInUTCTime)

	keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.PREPARE_REPORT, chatID)
	telegram_connector.SendMessage(chatID, message, keyboardIdentifier)
}

/*
HandleLengthReport is the function called on the actual Lx length report. It handles validation
and storage of the given report, as well as the feedback message.
*/
func HandleLengthReport(sentMessage string, messageUnixTime int, chatID int) {
	if reportAppearsValid(sentMessage) {
		errorWhileSaving := saveQueueLength(sentMessage, messageUnixTime, chatID)
		if errorWhileSaving == nil {
			if db_connectors.UserIsCollectingPoints(chatID) {
				db_connectors.AddInternetPoint(chatID)
			}
			sendThankYouMessage(chatID, sentMessage)
		}
	} else {
		sendNoThanksMessage(chatID, sentMessage)
	}

}

/*
   Sends a thank you message for a report
*/
func sendThankYouMessage(chatID int, textSentByUser string) {
	emojiRune := GetRandomAcceptableEmoji()
	baseMessage := "You reported length %s, thanks " + string(emojiRune)

	zap.S().Infof("Sending thank you for %s", textSentByUser)

	keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.LENGTH_REPORT, chatID)
	err := telegram_connector.SendMessage(chatID, fmt.Sprintf(baseMessage, textSentByUser), keyboardIdentifier)
	if err != nil {
		zap.S().Error("Error while sending thank you message.", err)
	}
}

func sendNoThanksMessage(chatID int, textSentByUser string) {
	emojiRune := GetRandomAcceptableEmoji()
	baseMessage := "...are you sure?" + string(emojiRune)

	zap.S().Infof("Sending no thanks for %s", textSentByUser)

	keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.LENGTH_REPORT, chatID)
	err := telegram_connector.SendMessage(chatID, baseMessage, keyboardIdentifier)
	if err != nil {
		zap.S().Error("Error while sending no thanks message.", err)
	}
}

/*
   Writes the given queue length to the database
*/
func saveQueueLength(queueLength string, unixTimestamp int, chatID int) error {
	chatIDString := strconv.Itoa(chatID)
	return db_connectors.WriteReportToDB(chatIDString, unixTimestamp, queueLength)
}

func reportAppearsValid(reportText string) bool {
	// Checking time: It's not on the weekend
	if utils.IsInDebugMode() {
		zap.S().Info("Running in Debug mode, skipping report validity check")
		return true
	}
	var today = time.Now()

	if today.Weekday() == 0 || today.Weekday() == 6 {
		// Sunday or Saturday, per https://golang.google.cn/pkg/time/#Weekday
		zap.S().Info("Report is on weekend")
		return false
	}

	if utils.GetMensaOpeningTime().After(today) ||
		utils.GetMensaClosingTime().Before(today) {
		zap.S().Info("Report is outside of mensa hours")
		// Outside of mensa closing times
		return false
	}
	zap.S().Info("Report is considered valid")
	return true
}
