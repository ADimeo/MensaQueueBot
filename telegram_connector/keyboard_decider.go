package telegram_connector

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"

	"go.uber.org/zap"
)

type UserRequestType string

// See more detailed descriptions within switch cases of
// GetIdentifierBasedOnInput()
const (
	LENGTH_REPORT        UserRequestType = "LENGTH_REPORT"
	PREPARE_REPORT       UserRequestType = "PREPARE_REPORT"
	INFO_REQUEST         UserRequestType = "INFO_REQUEST"
	PREPARE_SETTINGS     UserRequestType = "PREPARE_SETTINGS"
	SETTINGS_INTERACTION UserRequestType = "SETTINS_INTERACTION"
	PUSH_MESSAGE         UserRequestType = "PUSH_MESSAGE"
)

type KeyboardIdentifier int

const (
	LegacyKeyboard   KeyboardIdentifier = -1
	NilKeyboard      KeyboardIdentifier = 0
	ReportKeyboard   KeyboardIdentifier = 1
	MainKeyboard     KeyboardIdentifier = 2
	SettingsKeyboard KeyboardIdentifier = 3
)

const LEGACY_KEYBOARD_FILEPATH = "./keyboard.json"
const REPORT_KEYBOARD_FILEPATH = "./keyboards/00_report_keyboard.json"
const MAIN_KEYBOARD_FILEPATH = "./keyboards/01_main_keyboard.json"
const SETTINGS_KEYBOARD_FILEPATH = "./keyboards/02_settings_keyboard.json"

/*
Takes enum values, as defined in keyboard_decider.go
Reads a JSON file and returns a keyboard struct, depending on the requested identifier.
Raises error on unknown keyboard, or if a nil keyboard was requested - in that case
the caller needs to have special logic.

This function mostly exists to provide some stability for the NilKeyboard case.
Not having this function would lead to some weird things being encoded in
the compined GetIdentifierViaRequestType and GetKeyboardFromIdentifier function
*/
func GetKeyboardFromIdentifier(identifier KeyboardIdentifier) (ReplyKeyboardMarkupStruct, error) {
	switch identifier {
	case LegacyKeyboard:
		{
			return getReplyKeyboard(LEGACY_KEYBOARD_FILEPATH), nil
		}
	case NilKeyboard:
		{
			var nilKeyboard ReplyKeyboardMarkupStruct
			return nilKeyboard, errors.New("Caller requested nil keyboard, should not send keyboard instead")
		}
	case ReportKeyboard:
		{
			return getReplyKeyboard(REPORT_KEYBOARD_FILEPATH), nil
			return get
		}
	case MainKeyboard:
		{
			return getReplyKeyboard(MAIN_KEYBOARD_FILEPATH), nil
		}
	case SetingsKeyboard:
		{
			return getReplyKeyboard(SETTINGS_KEYBOARD_FILEPATH), nil
		}
	}
	var nilKeyboard ReplyKeyboardMarkupStruct
	return nilKeyboard, errors.New("Caller requested unknown keyboard type")
}

/* Function that takes a requestType enum and returns a keyboard enum, that is then looked up
when a keyboard should be used.
In theory each messageHandler already has the knowledge to call SendMessage by themselves,
a single response handler will very seldomly (never?) send more than one keyboard to the
user. However, I hope that this centralised switch statement will simplify maintenance, since
there's now a single point where this new functionality can be added, and no one will have
to look up "uuuhhh, which message sent that type again?"

*/
func GetIdentifierViaRequestType(requestType UserRequestType) (KeyboardIdentifier, error) {
	switch requestType {
	case LENGTH_REPORT:
		{
			// User sent a length report, or declined to report length. Always comes from the REPORT_KEYBOARD, and always leads to MAIN_KEYBOARD
			return MainKeyboard, nil
		}
	case PREPARE_REPORT:
		{
			// User wants to send a report/navigate to report keyboard. Always comes fom MAIN_KEYBOARD, always leads to REPORT_KEYBOARD
			return ReportKeyboard, nil
		}
	case INFO_REQUEST:
		{
			// User requested mensa menu or queue length. Always comes from MAIN_KEYBOARD, always stays at MAIN_KEYBOARD
			return NilKeyboard, nil
		}
	case PREPARE_SETTINGS:
		{
			// User wants to navigate to settings screen. Always comes from MAIN_KEYBOARD, always leads to SETTINGS_KEYBOARD
			return SettingsKeyboard, nil
		}
	case SETTINGS_INTERACTION:
		{
			// User did something in settings, either change settings, or request help. Always comes from SETTINGS_KEYBOARD, always stays at SETTINGS_KEYBOARD
			return SettingsKeyboard, nil
		}
	case PUSH_MESSAGE:
		{
			// A message the user didn't actively request. Always stays at current keyboard
			return NilKeyboard, nil
		}
	}
	return NilKeyboard, errors.New("Caller provided unknown requestType", requestType)
}

/*
- SendStaticWebPhoto
- SendDynamicPhoto
- SendMessage


TODO
Methods that send:
- SendStaticWebPhoto
	- Platypus bonus - no keyboard
	- introduction messages - nav back to main keyboard (optional?)
	- Send old QueueLengthReport - stay on main
- SendDynamicPhoto
	- newly generated graph - stay on main
- SendMessage
	- Changelogs - no change
	- Help message points - stay on options
	- Points_handler - potentially cut out completely into "settings" message?
		- Opt in/Out Points - stay on options, or back to main?
			- Will probably be cut completely
		- "You have collected X points"
	- reports_handler
		- thank you post report - back to main
		- no thanks - back to main
	- settings_handler
		- Accout deletion - remove all keyboards
		- AB Joining - main keyboard
	- requests_handler
		- queue length- stay in main
	- mensa scraper things - stay at same
	- introduction - main keyboard


TODO- add "resize" flag?
-
*/

// Returns the struct that represents the custom keyboard that should be shown to the user
// Reads the json from the given file
func getReplyKeyboard(jsonPath string) *ReplyKeyboardMarkupStruct {
	var keyboardArray [][]KeyboardButton

	jsonFile, err := os.Open(jsonPath)
	if err != nil {
		zap.S().Panicf("Can't access keyboard json file at %s", jsonPath)
	}
	defer jsonFile.Close()

	jsonAsBytes, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		zap.S().Panicf("Can't read keyboard json file at %s", jsonPath)
	}
	err = json.Unmarshal(jsonAsBytes, &keyboardArray)
	if err != nil {
		zap.S().Panicf("Keyboard json file not formatted correctly: %s", err)
	}

	keyboardStruct := ReplyKeyboardMarkupStruct{
		Keyboard: keyboardArray,
	}
	return &keyboardStruct
}