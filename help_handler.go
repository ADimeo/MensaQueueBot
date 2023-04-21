package main

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"go.uber.org/zap"
)

func SendHelpMessage(chatID int) {
	var helpMessageArray = [...]string{
		"Alright, I'll try to give you a detailed overview. Remember, for questions or other uncertainties either talk to @adimeo, or go directly to https://github.com/ADimeo/MensaQueueBot",
		"Let's start with length reports. You can report lengths via choosing \"Report!\", and then tapping one of the buttons. This information is aggregated, and distributed to all users that ask for \"Queue?\".",
		"If you're uncertain about which button corresponds to which queue length use /length_illustrations",
		"To receive mensa menus, you have two options. First, you can receive the latest menu by using \"Menu?\"",
		"Second, you can use /settings to define on which days and at which times you want to be informed about menu changes. This works much like the other mensa bots: At the dedicated time you receive a message that contains whatever is on offer at that specific time.",
		"Once you have reported a queue length these automatic updates stop for the day, since we assume that you won't care about what the mensa has on offer once you've already eaten.",
		"To suggest changes for how the bot behaves check out https://github.com/ADimeo/MensaQueueBot, or write to @adimeo directly.",
		"When in doubt check your /settings, and again, @adimeo is responsible for user satisfaction, so go and bother him if something is weird or doesn't work.",
	}

	for _, messageString := range helpMessageArray {
		keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.SETTINGS_INTERACTION, chatID)
		err := telegram_connector.SendMessage(chatID, messageString, keyboardIdentifier)
		if err != nil {
			zap.S().Error("Error while sending help messages.", err)
		}
	}
}

/*
   Returns a slice that contains a number of links to photographs and corresponding messages
   encoded within a mensaLocation struct.

   Should be sent together with the texts defined in getWelcomeMessageArray
   (except for the last queuelength entry "even longer", which doesn't have an image)
   The specific logic of how these two interact is encoded within sendWelcomeMessage

   The messages defined for these need to be consistent with the keyboard defined in ./keyboard.json, which is used by telegram_connector.go,
   as well as with the regex REPORT_REGEX that is used to identify the type of inbound messages in reactToRequest

*/
func GetMensaLocationSlice() *[]mensaLocation {
	var mensaLocationArray []mensaLocation

	// Read these from json file
	jsonFile, err := os.Open(MENSA_LOCATION_JSON_LOCATION)
	if err != nil {
		zap.S().Panicf("Can't access mensa locations json file at %s", MENSA_LOCATION_JSON_LOCATION)
	}
	defer jsonFile.Close()

	jsonAsBytes, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		zap.S().Panicf("Can't read mensa locations json file at %s", MENSA_LOCATION_JSON_LOCATION)

	}
	err = json.Unmarshal(jsonAsBytes, &mensaLocationArray)
	if err != nil {
		zap.S().Panicf("Mensa location json file is malformed, at %s", MENSA_LOCATION_JSON_LOCATION)
	}

	return &mensaLocationArray
}
