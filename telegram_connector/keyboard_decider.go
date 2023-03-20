package telegram_connector

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
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
	TUTORIAL_MESSAGE     UserRequestType = "TUTORIAL_MESSAGE"
	PREPARE_MAIN         UserRequestType = "PREPARE_MAIN"
	ACCOUNT_DELETION     UserRequestType = "ACCOUNT_DELETION"
)

type KeyboardIdentifier int

const (
	LegacyKeyboard   KeyboardIdentifier = -1
	NilKeyboard      KeyboardIdentifier = 0
	ReportKeyboard   KeyboardIdentifier = 1
	MainKeyboard     KeyboardIdentifier = 2
	SettingsKeyboard KeyboardIdentifier = 3
	NoKeyboard       KeyboardIdentifier = 4
)

const LEGACY_KEYBOARD_FILEPATH = "./telegram_connector/keyboards/keyboard.json"
const REPORT_KEYBOARD_FILEPATH = "./telegram_connector/keyboards/00_report_keyboard.json"
const MAIN_KEYBOARD_FILEPATH = "./telegram_connector/keyboards/01_main_keyboard.json"
const SETTINGS_KEYBOARD_FILEPATH = "./telegram_connector/keyboards/02_settings_keyboard.json"

// Needs to be consistent with javascript logic in settings.html
const KEYBOARD_SETTINGS_OPENER_BASE_QUERY_STRING = "?reportAtAll=%t&reportingDays=%d&fromTime=%d&toTime=%d"

func GetCustomizedKeyboardFromIdentifier(chatID int, identifier KeyboardIdentifier) (*ReplyKeyboardMarkupStruct, error) {
	baseKeyboard, err := getBaseKeyboardFromIdentifier(identifier)
	if err != nil {
		return baseKeyboard, err
	}
	return customizeKeyboardForUser(chatID, identifier, baseKeyboard)

}

/*
Takes enum values, as defined in keyboard_decider.go
Reads a JSON file and returns a keyboard struct, depending on the requested identifier.
Raises error on unknown keyboard, or if a nil keyboard was requested - in that case
the caller needs to have special logic.

This function mostly exists to provide some stability for the NilKeyboard case.
Not having this function would lead to some weird things being encoded in
the compined GetIdentifierViaRequestType and getBaseKeyboardFromIdentifier function
*/
func getBaseKeyboardFromIdentifier(identifier KeyboardIdentifier) (*ReplyKeyboardMarkupStruct, error) {
	switch identifier {
	case LegacyKeyboard:
		{
			return getReplyKeyboard(LEGACY_KEYBOARD_FILEPATH), nil
		}
	case NilKeyboard:
		{
			var nilKeyboard ReplyKeyboardMarkupStruct
			return &nilKeyboard, errors.New("Caller requested nil keyboard, should not send keyboard instead")
		}
	case NoKeyboard:
		{
			var noKeyboard ReplyKeyboardMarkupStruct
			return &noKeyboard, errors.New("Caller requested no keyboard, should send different message instead")
		}
	case ReportKeyboard:
		{
			return getReplyKeyboard(REPORT_KEYBOARD_FILEPATH), nil
		}
	case MainKeyboard:
		{
			return getReplyKeyboard(MAIN_KEYBOARD_FILEPATH), nil
		}
	case SettingsKeyboard:
		{
			return getReplyKeyboard(SETTINGS_KEYBOARD_FILEPATH), nil
		}
	}
	var nilKeyboard ReplyKeyboardMarkupStruct
	return &nilKeyboard, errors.New("Caller requested unknown keyboard type")
}

/*
"Customizes" the base keyboard for a single user. Right now this means exactly one thing:
The settings keyboard is enriched with the current user settings, so that they can be
displayed without serving an additional request.
*/
func customizeKeyboardForUser(userID int, identifier KeyboardIdentifier, baseKeyboard *ReplyKeyboardMarkupStruct) (*ReplyKeyboardMarkupStruct, error) {
	if identifier == SettingsKeyboard {
		// This is the only one that needs customization right now
		// We need to add the users current settings to the web_app url
		// which opens the webview with the settings
		settingsQueryString, err := getSettingsQueryStringForUser(userID)
		if err != nil {
			// TODO

		}
		customizedURL := baseKeyboard.Keyboard[1][1].WebApp.URL + settingsQueryString

		baseKeyboard.Keyboard[1][1].WebApp.URL = customizedURL
	}
	return baseKeyboard, nil
}

func getSettingsQueryStringForUser(userID int) (string, error) {
	preferencesStruct, err := db_connectors.GetUserPreferences(userID)
	if err != nil {
		// TODO
	}
	queryString := fmt.Sprintf(KEYBOARD_SETTINGS_OPENER_BASE_QUERY_STRING, preferencesStruct.ReportAtAll, preferencesStruct.WeekdayBitmap, preferencesStruct.FromTime, preferencesStruct.ToTime)
	return queryString, nil

}

/* Function that takes a requestType enum and returns a keyboard enum, that is then looked up
when a keyboard should be used.
In theory each messageHandler already has the knowledge to call SendMessage by themselves,
a single response handler will very seldomly (never?) send more than one keyboard to the
user. However, I hope that this centralised switch statement will simplify maintenance, since
there's now a single point where this new functionality can be added, and no one will have
to look up "uuuhhh, which message sent that type again?"
*/
func GetIdentifierViaRequestType(requestType UserRequestType, userID int) KeyboardIdentifier {
	userIsABTester := db_connectors.GetIsUserABTester(userID)
	zap.S().Debugf("User is sending us a %s request, returning corresponding keyboard identifier", requestType)
	if !userIsABTester {
		zap.S().Debug("Returning legacy keyboard, no AB Tester")
		return LegacyKeyboard
	}
	switch requestType {
	case LENGTH_REPORT:
		{
			// User sent a length report, or declined to report length. Always comes from the REPORT_KEYBOARD, and always leads to MAIN_KEYBOARD
			return MainKeyboard
		}
	case PREPARE_REPORT:
		{
			// User wants to send a report/navigate to report keyboard. Always comes fom MAIN_KEYBOARD, always leads to REPORT_KEYBOARD
			return ReportKeyboard
		}
	case INFO_REQUEST:
		{
			// User requested mensa menu or queue length. Always comes from MAIN_KEYBOARD, always stays at MAIN_KEYBOARD
			return NilKeyboard
		}
	case PREPARE_SETTINGS:
		{
			// User wants to navigate to settings screen. Always comes from MAIN_KEYBOARD, always leads to SETTINGS_KEYBOARD
			return SettingsKeyboard
		}
	case SETTINGS_INTERACTION:
		{
			// User did something in settings, either change settings, or request help. Always comes from SETTINGS_KEYBOARD, always stays at SETTINGS_KEYBOARD
			return SettingsKeyboard
		}
	case PUSH_MESSAGE:
		{
			// A message the user didn't actively request. Always stays at current keyboard
			return NilKeyboard
		}
	case TUTORIAL_MESSAGE:
		{
			// User requested help or a tutorial. Always stays at current keyboard
			return NilKeyboard
		}
	case PREPARE_MAIN:
		{
			// User wants to navigate to main. Usually starts at settings, always leads to main
			return MainKeyboard
		}
	case ACCOUNT_DELETION:
		// User deleted their account. Remove keyboard to make them feel like the interaction ended
		return NoKeyboard
	}
	zap.S().Error("Caller requested unknown keyboard type, returning nil keyboard")
	return NilKeyboard
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

func LoadAllKeyboardsForTest() {
	getBaseKeyboardFromIdentifier(LegacyKeyboard)
	getBaseKeyboardFromIdentifier(ReportKeyboard)
	getBaseKeyboardFromIdentifier(MainKeyboard)
	getBaseKeyboardFromIdentifier(SettingsKeyboard)
}
