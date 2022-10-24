package main

import (
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"
)

func getMensaOpeningTime() time.Time {
	var today = time.Now()
	// Mensa opens at 08:00
	var openingTime = time.Date(today.Year(), today.Month(), today.Day(), 8, 0, 0, 0, GetLocalLocation())
	return openingTime
}

func getMensaClosingTime() time.Time {
	var today = time.Now()
	// Mensa closes at 15:00
	var closingTime = time.Date(today.Year(), today.Month(), today.Day(), 15, 0, 0, 0, GetLocalLocation())
	return closingTime
}

/*
   Sends a thank you message for a report
*/
func sendThankYouMessage(chatID int, textSentByUser string) {
	emojiRune := GetRandomAcceptableEmoji()
	baseMessage := "You reported length %s, thanks " + string(emojiRune)

	zap.S().Infof("Sending thank you for %s", textSentByUser)

	err := SendMessage(chatID, fmt.Sprintf(baseMessage, textSentByUser))
	if err != nil {
		zap.S().Error("Error while sending thank you message.", err)
	}
}

func sendNoThanksMessage(chatID int, textSentByUser string) {
	emojiRune := GetRandomAcceptableEmoji()
	baseMessage := "...are you sure?" + string(emojiRune)

	zap.S().Infof("Sending no thanks for %s", textSentByUser)

	err := SendMessage(chatID, baseMessage)
	if err != nil {
		zap.S().Error("Error while sending no thanks message.", err)
	}
}

/*
   Writes the given queue length to the database
*/
func saveQueueLength(queueLength string, unixTimestamp int, chatID int) error {
	chatIDString := strconv.Itoa(chatID)
	return WriteReportToDB(chatIDString, unixTimestamp, queueLength)
}

func reportAppearsValid(reportText string) bool {
	// Checking time: It's not on the weekend
	var today = time.Now()

	if today.Weekday() == 0 || today.Weekday() == 6 {
		// Sunday or Saturday, per https://golang.google.cn/pkg/time/#Weekday
		zap.S().Info("Report is on weekend")
		return false
	}

	if getMensaOpeningTime().After(today) ||
		getMensaClosingTime().Before(today) {
		zap.S().Info("Report is outside of mensa hours")
		// Outside of mensa closing times
		return false
	}
	zap.S().Info("Report is considered valid")
	return true
}

func HandleLengthReport(sentMessage string, messageUnixTime int, chatID int) {
	if reportAppearsValid(sentMessage) {
		errorWhileSaving := saveQueueLength(sentMessage, messageUnixTime, chatID)
		if errorWhileSaving == nil {
			if UserIsCollectingPoints(chatID) {
				AddInternetPoint(chatID)
			}
			sendThankYouMessage(chatID, sentMessage)
		}
	} else {
		sendNoThanksMessage(chatID, sentMessage)
	}

}
