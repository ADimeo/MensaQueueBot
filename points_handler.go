package main

import (
	"fmt"

	"go.uber.org/zap"
)

func sendPointsHelpMessages(chatID int) {
	var messageArray = [...]string{
		"If you want to, you can opt in to collect internetpoints for your reports!",
		"You get one point for each report, and your points will add up with each report you make",
		"Here at MensaQueueBot, we try to minimize the data we collect. Right now all your reports are anonymized. Your reports will stay anonymous regardless of whether you collect points or not, but if you opt in we'll need to store additional information, specifically how many reports you've made. Just wanted to let you know that.",
		"Right now points don't do anything except prove to everybody what a great reporter you are, but we have plans for the future! (Maybe!)",
		`To start collecting points send /points_track`,
		`To stop colletcing points and delete all data related to point collection send /points_delete`,
		`To see your points send /points`,
	}
	for i := 0; i < len(messageArray); i++ {
		messageString := messageArray[i]
		err := SendMessage(chatID, messageString)
		if err != nil {
			zap.S().Error("Error while sending help message for point", err)
		}
	}
}

func sendPointsOptInResponse(chatID int, currentlyOptedIn bool) {
	messageOptIn := "Alrighty, from now on you're collecting points 🧞"
	messageDoubleOptIn := "Sure, but you were already collecting points 🧞"

	var err error
	if currentlyOptedIn {
		err = SendMessage(chatID, messageDoubleOptIn)
	} else {
		err = SendMessage(chatID, messageOptIn)
	}
	if err != nil {
		zap.S().Error("Error while sending points opt-in message.", err)
	}
}
func sendPointsOptOutResponse(chatID int, currentlyOptedIn bool) {
	messageOptOut := "You're the boss, all your points have been deleted 🥷"
	messageDoubleOptOut := "There's nothing to delete: You weren't collecting points 🥷"

	var err error
	if currentlyOptedIn {
		err = SendMessage(chatID, messageOptOut)
	} else {
		err = SendMessage(chatID, messageDoubleOptOut)
	}
	if err != nil {
		zap.S().Error("Error while sending points opt-out message.", err)
	}
}

func sendPointsRequestedResponse(chatID int, currentlyOptedIn bool, points int) error {
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

	explanationMessage := `You're currently not collecting points, but please know that we greatly appreciate all reports. For information about points send /points_help`

	var err error
	zap.S().Info("Sending pointsrequest message.")
	if currentlyOptedIn {
		pointsCollected := GetNumberOfPointsByUser(chatID)
		encouragementSelector := pointsCollected / 9 // New encouragement message every 9 points
		if encouragementSelector >= len(encouragements) {
			encouragementSelector = len(encouragements) - 1
		}

		encouragementMessage := encouragements[encouragementSelector]
		messageToSend := fmt.Sprintf(baseMessage, pointsCollected, encouragementMessage)
		err := SendMessage(chatID, messageToSend)
		if err != nil {
			zap.S().Errorf("Error while sending pointsrequest message for %s points", points, err)
		}
	} else {
		err = SendMessage(chatID, explanationMessage)
		if err != nil {
			zap.S().Error("Error while sending pointsrequest message.", err)
		}
	}
	return err
}

func HandlePointsRequest(sentMessage string, chatID int) {
	userIsCollectingPoints := UserIsCollectingPoints(chatID)

	if sentMessage == "/points" {
		points := 0
		if userIsCollectingPoints {
			points = GetNumberOfPointsByUser(chatID)
		}
		sendPointsRequestedResponse(chatID, userIsCollectingPoints, points)
	} else if sentMessage == "/points_track" {
		if userIsCollectingPoints {
			// Nothing to do: User is already opted in
		} else {
			EnableCollectionOfPoints(chatID)
		}
		sendPointsOptInResponse(chatID, userIsCollectingPoints)
	} else if sentMessage == "/points_delete" {
		if userIsCollectingPoints {
			DisableCollectionOfPoints(chatID)
		} else {
			// Nothing to do: User is already opted out
		}
		sendPointsOptOutResponse(chatID, userIsCollectingPoints)
	} else if sentMessage == "/points_help" {
		sendPointsHelpMessages(chatID)
	} else {
		zap.S().Infof("Usermessage '%s' does not match with any point message", sentMessage)
	}
}
