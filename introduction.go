package main

/*
Functions that are used to send out the introduction messages (/start)
These contain hardcoded business logic
*/

import (
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"go.uber.org/zap"
)

const TOP_VIEW_URL = "https://raw.githubusercontent.com/ADimeo/MensaQueueBot/master/queue_length_illustrations/top_view.jpg"

/*
   Contains a number of messages that should be sent to users as an introduction.
   Should be sent together with the image (links) defined in GetMensaLocationSlice.

   The specific logic of how these two interact is encoded within SendWelcomeMessage
*/

func getWelcomeMessageArray() [5]string {
	var messageArray = [...]string{
		"Welcome to MensaQueueBot 2.0, where we minimize wait times, and maximize food enjoyment for you! I'll quickly get you onboarded, if you don't mind.",
		"Right now we offer two functionalities: First, crowdsourced mensa lengths. Request past and current lengths of the mensa queue via the \"Queue?\" button, and report queue lengths via \"Report!",
		// Send top picture here
		"If the queue ends before the line marked as L3 report the length as L3",
		"Second, we offer the current mensa menu, including changes which happened during the day. To get the latest menu use the \"Menu?\" button.",
		"That's all for now. Check out /settings for additional information, including changing when you receive menu updates, internet points, and more details on length reporting.",
	}
	return messageArray
}

func SendTopViewOfMensa(chatID int) error {
	const linkToTopView = "https://raw.githubusercontent.com/ADimeo/MensaQueueBot/master/queue_length_illustrations/top_view.jpg"
	const topViewText = "I'm an artist"
	keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.IMAGE_REQUEST, chatID)
	err := telegram_connector.SendStaticWebPhoto(chatID, TOP_VIEW_URL, topViewText, keyboardIdentifier)
	return err
}

/*
   Sends a number of messages to the specified user, explaining the base concept and instructing them on how to act
   Tightly coupled with getWelcomeMessageArray and GetMensaLocationSlice
*/
func SendWelcomeMessage(chatID int) {
	messageArray := getWelcomeMessageArray()

	var err error
	// Send first two messages
	for i := 0; i < 2; i++ {
		messageString := messageArray[i]
		keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.TUTORIAL_MESSAGE, chatID)
		err = telegram_connector.SendMessage(chatID, messageString, keyboardIdentifier)
		if err != nil {
			zap.S().Error("Error while sending first welcome messages.", err)
		}

	}
	// Send Top view of mensa
	err = SendTopViewOfMensa(chatID)
	if err != nil {
		zap.S().Error("Error while sending Top View of mensa.", err)
	}

	for i := 2; i < 5; i++ {
		messageString := messageArray[i]
		keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.TUTORIAL_MESSAGE, chatID)
		err = telegram_connector.SendMessage(chatID, messageString, keyboardIdentifier)
		if err != nil {
			zap.S().Error("Error while sending second welcome messages.", err)
		}
	}

}
