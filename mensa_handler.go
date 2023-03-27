package main

import (
	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/utils"
	"go.uber.org/zap"
)

// Temporary function that adds user to mensa receivers list (which we are currently A/B Testing)
func ABTestHandler(userID int) {
	// Adds them all day every day to all messages
	var err error
	if utils.IsInDebugMode() {
		err = db_connectors.UpdateUserPreferences(userID, true, 0, 1440, 0b0111110) // Default from 0:00 to 24:00
	} else {
		err = db_connectors.UpdateUserPreferences(userID, true, 600, 840, 0b0111110) // Default from 10:00 to 14:00
	}

	if err != nil {
		zap.S().Error(err)

	}

}
