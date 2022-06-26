package main

/*
Functions that store _some_ knowledge (like messages, or in which specific order messages
should be sent) that really should be stored better), or functions that act as static variables.
*/

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

	"go.uber.org/zap"
)

const TOP_VIEW_URL = "https://raw.githubusercontent.com/ADimeo/MensaQueueBot/master/queue_length_illustrations/top_view.jpg"

/*
   Contains a number of messages that should be sent to users as an introduction.
   Should be sent together with the image (links) defined in GetMensaLocationSlice.

   The specific logic of how these two interact is encoded within sendWelcomeMessage
*/
func getWelcomeMessageArray() [9]string {
	var messageArray = [...]string{
		"Thanks for joining the wait-less-for-food initiative! I'll quickly get you onboarded, if you don't mind",
		"Your assignment is to report the length of the mensa queue. To simplify reporting we have assigned different IDs to different queue lengths. For example:",
		// Send picture here
		"If the mensa line ends before this red line you'd report it as \"L3: Within first room\"",
		"To report a length you tap on the buttons displayed in this chat. In total, we have defined 8 queue length segments, so you have 9 reporting buttons available - the catchall \"even longer\" is not explicitly illustrated",
		"The different line segments are the following:",
		// Send top picture here
		"(If you want a better illustration of line lengths you can use /help)",
		"Once we have collected enough data we'll provide you with an overview of when on which days the mensa queue is shortest - that means you'll waste less time just standing in line",
		`You can also use /jetze to find out what length the mensa queue has right now, and if you're interested in getting points for your reports be sure to check out /points_help`,
		"If you have any additional questions feel free to ask @adimeo. For everything else the repository for this bot is available at https://github.com/ADimeo/MensaQueueBot",
	}
	return messageArray
}

/*
   Returns a slice that contains a number of links to photographs and corresponding messages
   encoded within a mensaLocation struct.

   Should be sent together with the texts defined in getWelcomeMessageArray
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

func GetLocalLocation() *time.Location {
	potsdamLocation, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		zap.S().Panic("Can't load location Europe/Berlin!")
	}
	return potsdamLocation
}

func GetMensaOpeningTime() time.Time {
	var today = time.Now()
	// Mensa opens at 08:00
	var openingTime = time.Date(today.Year(), today.Month(), today.Day(), 8, 0, 0, 0, GetLocalLocation())
	return openingTime
}

func GetMensaClosingTime() time.Time {
	var today = time.Now()
	// Mensa closes at 15:00
	var closingTime = time.Date(today.Year(), today.Month(), today.Day(), 15, 0, 0, 0, GetLocalLocation())
	return closingTime
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

func SendTopViewOfMensa(chatID int) error {
	const linkToTopView = "https://raw.githubusercontent.com/ADimeo/MensaQueueBot/master/queue_length_illustrations/top_view.jpg"
	const topViewText = "I'm an artist"
	err := SendPhoto(chatID, TOP_VIEW_URL, topViewText)
	return err
}

/*
   Sends a number of messages to the specified user, explaining the base concept and instructing them on how to act
   Tightly coupled with getWelcomeMessageArray and GetMensaLocationSlice
*/
func SendWelcomeMessage(chatID int) {
	messageArray := getWelcomeMessageArray()
	mensaLocationArray := *GetMensaLocationSlice()

	var err error
	// Send first two messages
	for i := 0; i < 2; i++ {
		messageString := messageArray[i]
		err = SendMessage(chatID, messageString)
		if err != nil {
			zap.S().Error("Error while sending first welcome messages.", err)
		}

	}

	// Send single photo for illustration
	err = SendPhoto(chatID, mensaLocationArray[3].PhotoUrl, mensaLocationArray[3].Description)
	if err != nil {
		zap.S().Error("Error while sending first welcome messages.", err)
	}

	for i := 2; i < 5; i++ {
		messageString := messageArray[i]
		err = SendMessage(chatID, messageString)
		if err != nil {
			zap.S().Error("Error while sending second welcome messages.", err)
		}
	}

	// Send Top view of mensa
	err = SendTopViewOfMensa(chatID)
	if err != nil {
		zap.S().Error("Error while sending Top View of mensa.", err)
	}

	for i := 5; i < 9; i++ {
		messageString := messageArray[i]
		err = SendMessage(chatID, messageString)
		if err != nil {
			zap.S().Error("Error while sending final welcome messages.", err)
		}
	}
}
