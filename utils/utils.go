package utils

import (
	"os"
	"time"

	"go.uber.org/zap"
)

const KEY_PERSONAL_TOKEN string = "MENSA_QUEUE_BOT_PERSONAL_TOKEN"

func GetLocalLocation() *time.Location {
	potsdamLocation, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		zap.S().Panic("Can't load location Europe/Berlin!")
	}
	return potsdamLocation
}

/*
   Reads the personal token from environment variables.
   The personal token is part of the url path, and tries to prevent non-authorized users from accessing our webhooks, and therefore spamming our users.
   For this purpose it needs to be long, random, and non-public.
*/
func GetPersonalToken() string {
	personalKey, doesExist := os.LookupEnv(KEY_PERSONAL_TOKEN)

	if !doesExist {
		zap.S().Panicf("Fatal Error: Environment variable for personal key not set: %s", KEY_PERSONAL_TOKEN)
	}
	return personalKey
}
