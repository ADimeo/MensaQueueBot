package telegram_connector

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

// Used for "typing..." indicators,
// https://core.telegram.org/bots/api#sendchataction
type sendChatActionRequestBody struct {
	ChatID int    `json:"chat_id"`
	Action string `json:"action"`
}

type WebAppInfo struct {
	URL string `json:"url"`
}

type KeyboardButton struct {
	Text   string      `json:"text"`
	WebApp *WebAppInfo `json:"web_app,omitempty"`
}

type ReplyKeyboardMarkupStruct struct { // https://core.telegram.org/bots/api/#replykeyboardmarkup
	Keyboard [][]KeyboardButton `json:"keyboard"`
}

// Used for images whose ID or URL we have.
// No specific struct exists for "dynamic"
// image uploads
type sendWebPhotoRequestBody struct {
	ChatID  int    `json:"chat_id"`
	Photo   string `json:"photo"`
	Caption string `json:"caption"`
}

type telegramResponseBodyPhoto struct {
	FileID string `json:"file_id"` // This is the ID we want to use to re-send this image
}

type telegramResponseBody struct {
	Result struct {
		Photo []telegramResponseBodyPhoto `json:"photo"`
	} `json:"result"`
}

// Returns the struct that represents the custom keyboard that should be shown to the user
func GetReplyKeyboard() *ReplyKeyboardMarkupStruct {
	var keyboardArray [][]KeyboardButton

	jsonFile, err := os.Open(KEYBOARD_FILE_LOCATION)
	if err != nil {
		zap.S().Panicf("Can't access keyboard json file at %s", KEYBOARD_FILE_LOCATION)
	}
	defer jsonFile.Close()

	jsonAsBytes, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		zap.S().Panicf("Can't read keyboard json file at %s", KEYBOARD_FILE_LOCATION)
	}
	err = json.Unmarshal(jsonAsBytes, &keyboardArray)
	if err != nil {
		zap.S().Panicf("Keyboard json file not formatted correctly: %s", err)
	}

	keyboardStruct := ReplyKeyboardMarkupStruct{
		Keyboard: keyboardArray,
	}
	// TODO Hacky temporary reply-keyboard thingy

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
	part, err := writer.CreateFormFile("photo", filepath.Base(pathToFile))
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

Returns telegram assigned identifier and error, if the request should fail
*/
func SendDynamicPhoto(chatID int, photoFilePath string, description string) (string, error) {
	telegramUrl := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", GetTelegramToken())

	requestBody, contentType, err := prepareMultipartForUpload(photoFilePath, chatID, description)
	if err != nil {
		zap.S().Errorf("Couldn't build request to send detailed /jetze report")
		return "", err
	}
	request, _ := http.NewRequest("POST", telegramUrl, requestBody)
	request.Header.Add("Content-Type", contentType)
	client := &http.Client{}
	response, err := client.Do(request)
	defer response.Body.Close()

	if err != nil {
		zap.S().Errorw("Dynamic photo request failed", "error", err)
		return "", err
	}
	telegramResponse := &telegramResponseBody{}
	responseDecoder := json.NewDecoder(response.Body)
	err = responseDecoder.Decode(telegramResponse)

	if err != nil {
		return "", err
	}

	// Telegram returns a list of images, in different resolutions
	// All of them share the same file_id
	telegramIdentifier := telegramResponse.Result.Photo[0].FileID

	return telegramIdentifier, nil
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

/* SendTypingIndicator sets the bots status to "sending image"
for this specific user*/
func SendTypingIndicator(chatID int) error {
	telegramUrl := fmt.Sprintf("https://api.telegram.org/bot%s/sendChatAction", GetTelegramToken())
	indicatorString := "upload_photo"
	requestBody := &sendChatActionRequestBody{
		ChatID: chatID,
		Action: indicatorString,
	}
	reqBytes, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}
	_, err = http.Post(telegramUrl, "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		zap.S().Error("Failure while sending typing indicator", err)
		return err
	}
	return nil
}
