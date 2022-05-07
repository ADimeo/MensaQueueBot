package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"go.uber.org/zap"
)

const KEY_TELEGRAM_TOKEN string = "MENSA_QUEUE_BOT_TELEGRAM_TOKEN"
const KEYBOARD_FILE_LOCATION = "./keyboard.json"

// Struct definitions taken from https://www.sohamkamani.com/golang/telegram-bot/
type WebhookRequestBody struct {
	Message struct {
		Text string `json:"text"`
		Chat struct {
			ID int `json:"id"`
		} `json:"chat"`
		Date int `json:"date"`
	} `json:"message"`
}

type sendMessageRequestBody struct {
	ChatID              int                       `json:"chat_id"`
	Text                string                    `json:"text"`
	ReplyKeyboardMarkup ReplyKeyboardMarkupStruct `json:"reply_markup"`
}

type ReplyKeyboardMarkupStruct struct { // https://core.telegram.org/bots/api/#replykeyboardmarkup
	Keyboard [][]string `json:"keyboard"` // Can be string per https://core.telegram.org/bots/api/#keyboardbutton
}

type sendPhotoRequestBody struct {
	ChatID  int    `json:"chat_id"`
	Photo   string `json:"photo"`
	Caption string `json:"caption"`
}

// Returns the struct that represents the custom keyboard that should be shown to the user
func GetReplyKeyboard() *ReplyKeyboardMarkupStruct {
	var keyboardArray [][]string

	jsonFile, err := os.Open(KEYBOARD_FILE_LOCATION)
	if err != nil {
		zap.S().Panicf("Can't access keyboard json file at %s", KEYBOARD_FILE_LOCATION)
	}
	defer jsonFile.Close()

	jsonAsBytes, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		zap.S().Panicf("Can't read keyboard json file at %s", KEYBOARD_FILE_LOCATION)
	}
	json.Unmarshal(jsonAsBytes, &keyboardArray)

	keyboardStruct := ReplyKeyboardMarkupStruct{
		Keyboard: keyboardArray,
	}

	return &keyboardStruct

}

/*
   Reads the telegram token from an environment variable.
   The Telegram token is used to identify us to the telegram server when sending messages.
*/
func GetTelegramToken() string {
	telegramKey, doesExist := os.LookupEnv(KEY_TELEGRAM_TOKEN)
	if !doesExist {
		zap.S().Panicf("Error: Environment variable for personal key (%s) not set", KEY_TELEGRAM_TOKEN)
	}
	return telegramKey
}

/*
   Sends a POST request to the telegram API that contains the link to a photo. This photo is sent to the identified user. description is set as the text of the message
   https://core.telegram.org/bots/api#sendphoto
*/
func SendPhoto(chatID int, photoURL string, description string) error {
	telegramUrl := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", GetTelegramToken())

	requestBody := &sendPhotoRequestBody{
		ChatID:  chatID,
		Photo:   photoURL,
		Caption: description,
	}

	reqBytes, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	_, err = http.Post(telegramUrl, "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		return err
	}
	return nil
}

/*
   Sends a POST request to the telegram API that sends the indicated string to the indicated user.
   https://core.telegram.org/bots/api#sendmessage
*/
func SendMessage(chatID int, message string) error {
	telegramUrl := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", GetTelegramToken())
	keyboard := GetReplyKeyboard()

	requestBody := &sendMessageRequestBody{
		ChatID:              chatID,
		Text:                message,
		ReplyKeyboardMarkup: *keyboard,
	}

	reqBytes, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	_, err = http.Post(telegramUrl, "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		return err
	}
	return nil
}
