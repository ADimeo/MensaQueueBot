package main

import (
	"fmt"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"go.uber.org/zap"
)

/*
Sends a number of messages explaining how points work, used when requesting points help
*/
func SendPointsHelpMessages(chatID int) {
	var messageArray = [...]string{
		"If you want to, you can opt in to collect internetpoints for your reports!",
		"To opt into or out of point collection use the \"Change Settings\" button. Your points are displayed on the /settings screen.",
		"You get one point for each report, and your points will add up with each report you make!",
		"Here at MensaQueueBot, we try to minimize the data we collect. Right now all your reports are anonymized. Your reports will stay anonymous regardless of whether you collect points or not, but if you opt in we'll need to store additional information, specifically how many reports you've made. Just wanted to let you know that.",
		"Right now points don't do anything except prove to everybody what a great reporter you are, but we have plans for the future! (Maybe!)",
	}
	for i := 0; i < len(messageArray); i++ {
		messageString := messageArray[i]
		keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.SETTINGS_INTERACTION, chatID)
		err := telegram_connector.SendMessage(chatID, messageString, keyboardIdentifier)
		if err != nil {
			zap.S().Error("Error while sending help message for point", err)
		}
	}
}

/*
Returns a string which says how many points a user has,
but in pretty words
*/
func GetPointsRequestResponseText(chatID int) string {
	emojiRune := GetRandomAcceptableEmoji()
	baseMessage := "You have collected %d points%s" + string(emojiRune)
	var encouragements = [...]string{
		", that's a good start 🐨",
		", which is like two weeks of reporting every singe day 🏋️",
		", way to go! 🎯",
		". You can officially claim that you're a professional mensa queue length reporter, and I'll support that claim. 🌠",
		". Consider me impressed 🛍",
		". Do you always go above and beyond? 🛫",
		". Wow. 📸",
		", and I'll be honest, I don't know what to say 🪕",
	}

	explanationMessage := `You are currently not collecting points.`

	currentlyOptedIn := db_connectors.UserIsCollectingPoints(chatID)
	if !currentlyOptedIn {
		return explanationMessage
	}
	pointsCollected := db_connectors.GetNumberOfPointsByUser(chatID)
	encouragementSelector := pointsCollected / 9 // New encouragement message every 9 points
	if encouragementSelector >= len(encouragements) {
		encouragementSelector = len(encouragements) - 1
	}

	encouragementMessage := encouragements[encouragementSelector]
	messageToSend := fmt.Sprintf(baseMessage, pointsCollected, encouragementMessage)
	return messageToSend
}
