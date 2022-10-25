package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

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

// Used for images whose ID or URL we have.
// No specific struct exists for "dynamic"
// image uploads
type sendWebPhotoRequestBody struct {
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
   Sends a POST request to the telegram API that contains the link to a photo. This photo is sent to the identified user. Description is set as the text of the message
   https://core.telegram.org/bots/api#sendphoto
*/
func SendStaticWebPhoto(chatID int, photoURL string, description string) error {
	telegramUrl := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", GetTelegramToken())

	requestBody := &sendWebPhotoRequestBody{
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

/* PrepareMultipartForUpload reads the given file, chatID and caption, and writes them
to a buffer in a format corresponding to a multipart request.
Addicionally, it also returns the FormDataContentType for said multipart request.
*/
func prepareMultipartForUpload(pathToFile string, chatID int, caption string) (*bytes.Buffer, string, error) {
	// Read file content
	file, err := os.Open(pathToFile)
	defer file.Close()
	requestBody := new(bytes.Buffer)
	if err != nil {
		zap.S().Errorf("Can't open graph file for detailed /jetze report: %s", pathToFile)
		return requestBody, "", err
	}
	writer := multipart.NewWriter(requestBody)
	defer writer.Close()
	pathToFile.Base
	part, err := writer.CreateFormFile("photo", filepath.Base(filename))
	if err != nil {
		zap.S().Errorf("Can't CreateFormFile for /jetze report: %s", pathToFile)
		return nil, "", err
	}
	io.Copy(part, file)

	writer.WriteField("chat_id", strconv.Itoa(chatID))
	writer.WriteField("caption", caption)

	return requestBody, writer.FormDataContentType(), nil
}

/* SendDynamicPhoto sends an image that is stored locally on this machine
to the user with the given chatID. A description/caption can also be added.
*/
func SendDynamicPhoto(chatID int, photoFilePath string, description string) error {
	telegramUrl := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", GetTelegramToken())

	requestBody, contentType, err := prepareMultipartForUpload(photoFilePath, chatID, description)
	if err != nil {
		zap.S().Errorf("Couldn't build request to send detailed /jetze report")
		return err
	}
	request, _ := http.NewRequest("POST", telegramUrl, requestBody)
	request.Header.Add("Content-Type", contentType)
	client := &http.Client{}
	response, err := client.Do(request)

	if err != nil {
		zap.S().Errorw("Dynamic photo request failed", "error", err)
		return err
	}

	response.Location()
	return nil

	/*
		// TODO return and store this so we only need to send it once
		// Multiple photos of different sizes, but all have the same file_id
		Response.photo[0].file_id
	*/
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
