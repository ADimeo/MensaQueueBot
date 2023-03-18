package main

import (
	"io/ioutil"
	"math/rand"
	"os"
	"regexp"
	"time"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/mensa_scraper"
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"github.com/ADimeo/MensaQueueBot/utils"
	"github.com/gin-gonic/gin"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"go.uber.org/zap"
)

// const KEY_PERSONAL_TOKEN string = "MENSA_QUEUE_BOT_PERSONAL_TOKEN" // Defined in utils/utils.go, here for reference

const MENSA_LOCATION_JSON_LOCATION string = "./mensa_locations.json"

const REPORT_REGEX string = `^L\d: ` // A message that matches this regex is a length report, and should be treated as such
const POINTS_REGEX string = `^/points(_track|_delete|_help|)$`

var globalEmojiOfTheDay emojiOfTheDay

type mensaLocation struct {
	PhotoUrl    string `json:"photo_url"`
	Description string `json:"description"`
}

type emojiOfTheDay struct {
	Timestamp time.Time
	Emoji     rune
}

/*
   People like emoji. People also like slot machines. Return a random, pre-vetted emoji when they
   report, for "engagement"

    One emoji per day is chosen.
*/
func GetRandomAcceptableEmoji() rune {
	timestampNow := time.Now()
	dateToday := timestampNow.YearDay()
	if globalEmojiOfTheDay.Emoji == 0 || dateToday != globalEmojiOfTheDay.Timestamp.YearDay() {
		// Regenerate
		emoji_filepath := "./emoji_list"
		emojiFile, err := os.Open(emoji_filepath)
		if err != nil {
			zap.S().Errorf("Can't access emoji file at", emoji_filepath)
		}
		defer emojiFile.Close()

		emojiAsBytes, err := ioutil.ReadAll(emojiFile)
		if err != nil {
			zap.S().Errorf("Can't access emoji file at", emoji_filepath)
		}

		emojiRunesSlice := []rune(string(emojiAsBytes))
		pseudorandomPosition := rand.Intn(len(emojiRunesSlice))
		globalEmojiOfTheDay.Emoji = emojiRunesSlice[pseudorandomPosition]
		globalEmojiOfTheDay.Timestamp = timestampNow

	}
	return globalEmojiOfTheDay.Emoji
}

func parseRequest(c *gin.Context) (*telegram_connector.WebhookRequestBody, error) {
	body := &telegram_connector.WebhookRequestBody{}
	err := c.ShouldBind(&body)
	return body, err
}

func sendChangelogIfNecessary(chatID int) {
	numberOfLastSentChangelog := db_connectors.GetLatestChangelogSentToUser(chatID)
	changelog, noChangelogWithIDError := db_connectors.GetChangelogByNumber(numberOfLastSentChangelog + 1)

	if noChangelogWithIDError == nil {
		sendError := telegram_connector.SendMessage(chatID, changelog)
		if sendError == nil {
			db_connectors.SaveNewChangelogForUser(chatID, numberOfLastSentChangelog+1)
		} else {
			zap.S().Error("Got an error while sending changelog to user.", sendError)
		}
	}
}

func sendQueueLengthExamples(chatID int) {
	mensaLocationArray := *GetMensaLocationSlice()
	for _, mensaLocation := range mensaLocationArray {
		if mensaLocation.PhotoUrl != "" {
			err := telegram_connector.SendStaticWebPhoto(chatID, mensaLocation.PhotoUrl, mensaLocation.Description)
			if err != nil {
				zap.S().Error("Error while sending help message photographs.", err)
			}

		}
	}
	SendTopViewOfMensa(chatID)
}

func legacyRequestSwitch(chatID int, sentMessage string, bodyAsStruct *telegram_connector.WebhookRequestBody) {
	lengthReportRegex := regexp.MustCompile(REPORT_REGEX)
	pointsRegex := regexp.MustCompile(POINTS_REGEX)
	switch {
	case sentMessage == "/start":
		{
			zap.S().Info("Sending onboarding (/start) messages")
			SendWelcomeMessage(chatID)
		}
	case sentMessage == "/help":
		{
			zap.S().Info("Sending queue length (/help) messages")
			sendQueueLengthExamples(chatID)
		}
	case pointsRegex.Match([]byte(sentMessage)):
		{
			zap.S().Info("User is checking point status")
			HandlePointsRequest(sentMessage, chatID)
			sendChangelogIfNecessary(chatID)
		}
	case sentMessage == "/jetze":
		{
			zap.S().Infof("Received a /jetze request")
			GenerateAndSendGraphicQueueLengthReport(chatID)
			sendChangelogIfNecessary(chatID)
		}
	case sentMessage == "/jetze@MensaQueueBot":
		zap.S().Infof("Received a /jetze request, but in a group")
		GenerateAndSendGraphicQueueLengthReport(chatID)
		sendChangelogIfNecessary(chatID)
	case lengthReportRegex.Match([]byte(sentMessage)):
		{
			zap.S().Infof("Received a new report: %s", sentMessage)
			messageUnixTime := bodyAsStruct.Message.Date
			HandleLengthReport(sentMessage, messageUnixTime, chatID)
			sendChangelogIfNecessary(chatID)
		}
	case sentMessage == "/forgetme":
		{
			zap.S().Infof("User requested deletion of their data: %s", sentMessage)
			HandleAccountDeletion(chatID)
		}
	case sentMessage == "/joinABTesters": // In the future reading secret codes might be interesting
		{
			zap.S().Infof("User %d is joining test group", chatID)
			HandleABTestJoining(chatID)
		}
	case sentMessage == "": // this likely means that user used a keyboard-html-button thingy
		{
			zap.S().Debug("User is changing settings")
			HandleSettingsChange(chatID, bodyAsStruct.Message.WebAppData)
		}
	case sentMessage == "/platypus":
		{
			zap.S().Infof("PLATYPUS!")
			url := "https://upload.wikimedia.org/wikipedia/commons/4/4a/%22Nam_Sang_Woo_Safety_Matches%22_platypus_matchbox_label_art_-_from%2C_Collectie_NMvWereldculturen%2C_TM-6477-76%2C_Etiketten_van_luciferdoosjes%2C_1900-1949_%28cropped%29.jpg"
			telegram_connector.SendStaticWebPhoto(chatID, url, "So cute ❤️")
		}
	default:
		{
			zap.S().Infof("Received unknown message: %s", sentMessage)
		}
	}
}

func requestSwitch(chatID int, sentMessage string, bodyAsStruct *telegram_connector.WebookRequestBody) {
	lengthReportRegex := regexp.MustCompile(REPORT_REGEX)
	switch {
	// CASES FROM MAIN KEYBOARD
	case sentMessage == "Jetze?":
		{
			zap.S().Info("Received a 'Jetze?' request")
			GenerateAndSendGraphicQueueLengthReport(chatID)
			sendChangelogIfNecessary(chatID)
		}
	case sentMessage == "Menu?":
		{
			zap.S().Info("Received a 'Menu?' request")
			if err := mensa_scraper.SendLatestMenuToSingleUser(chatID); err != nil {
				message := "I'm so sorry, I can't find the current menu for today 🤕"
				keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.INFO_REQUEST, chatID)
				telegram_connector.SendMessage(chatID, message, keyboardIdentifier)

			}
			sendChangelogIfNecessary(chatID)
		}
	case sentMessage == "Report!":
		{
			zap.S().Info("Received a 'Report!' request")
			message := "Great! How is it looking?"
			telegram_connector.SendMessage(chatID, message)
		}
		// CASES FROM REPORT KEYBOARD
	case lengthReportRegex.Match([]byte(sentMessage)):
		{
			zap.S().Info("Received a new report: %s", sentMessage)
			messageUnixTime := bodyAsStruct.Message.Date
			HandleLengthReport(sentMessage, messageUnixTime, chatID)
			sendChangelogIfNecessary(chatID)
		}
	case sentMessage == "Can't tell":
		{
			zap.S().Info("Received a 'Can't tell' report")
			message := "Alrighty"
			telegram_connector.SendMessage(chatID, message)
			sendChangelogIfNecessary(chatID)
		}
		// CASES FROM SETTINGS KEYBOARD
	case sentMessage == "/settings":
		{
			// Let's not forget how to get to the settings screen...
			// TODO
			zap.S().Info("Received a '/settings' request")

			message := "TODO PLACEHOLDER SETTINGS REPORT W. POINTS, SETTINGS OVERVIEW, MENSABOT VERSION, WHETHER AB TESTER"
			telegram_connector.SendMessage(chatID, message)
		}
	case sentMessage == "General Help":
		{
			// Revamping this is contained within a different issue...
			zap.S().Info("Received a 'General Help' requets")
			sendQueueLengthExamples(chatID)
		}
	case sentMessage == "Points Help":
		{
			zap.S().Info("Received a 'Points Help' requets")
			SendPointsHelpMessages(chatID)
		}
	case sentMessage == "": // this likely means that user used a keyboard-html-button thingy
		{
			zap.S().Debug("User is changing settings")
			HandleSettingsChange(chatID, bodyAsStruct.Message.WebAppData)
		}
	case sentMessage == "Account Deletion":
		{
			// This is just for info
			zap.S().Info("Received a 'Account Deletion' request")
			message := "To delete all data about you from MensaQueueBot type /forgetme in the chat. Be advised that this action is destructive, and nonreversible. If you ever decide to come back you will be an entirely new user."
			telegram_connector.SendMessage(chatID, message)
		}
	case sentMessage == "Back":
		{
			zap.S().Info("Received a 'Back' report")
			message := "Back to my purpose"
			telegram_connector.SendMessage(chatID, message)
			sendChangelogIfNecessary(chatID)
		}
		// OTHER CASES
	case sentMessage == "/help":
		{
			zap.S().Info("Sending queue length (/help) messages")
			sendQueueLengthExamples(chatID)
		}
	case sentMessage == "/forgetme":
		{
			zap.S().Infof("User requested deletion of their data: %s", sentMessage)
			HandleAccountDeletion(chatID)
		}
	case sentMessage == "/joinABTesters": // In the future reading secret codes might be interesting
		{
			zap.S().Infof("User %d is joining test group", chatID)
			HandleABTestJoining(chatID)
		}
	case sentMessage == "/platypus":
		{
			zap.S().Infof("PLATYPUS!")
			url := "https://upload.wikimedia.org/wikipedia/commons/4/4a/%22Nam_Sang_Woo_Safety_Matches%22_platypus_matchbox_label_art_-_from%2C_Collectie_NMvWereldculturen%2C_TM-6477-76%2C_Etiketten_van_luciferdoosjes%2C_1900-1949_%28cropped%29.jpg"
			telegram_connector.SendStaticWebPhoto(chatID, url, "So cute ❤️")
		}
	default:
		{
			zap.S().Infof("Received unknown message: %s", sentMessage)
		}

	}

}

func reactToRequest(ginContext *gin.Context) {
	// Return some 200 or something

	bodyAsStruct, err := parseRequest(ginContext)
	if err == nil {
		ginContext.JSON(200, gin.H{
			"message": "Thanks nice server",
		})
	} else {
		zap.S().Error("Inbound data from telegram couldn't be parsed", err)
	}

	sentMessage := bodyAsStruct.Message.Text
	chatID := bodyAsStruct.Message.Chat.ID
	if db_connectors.IsUserABTester(chatID) {
		requestSwitch(chatID, sentMessage, bodyAsStruct)
	} else {
		legacyRequestSwitch(chatID, sentMessage, bodyAsStruct)
	}
}

/*
   Initiates our zap logger. We only log to Stdout, which our deployment setup will automatically forward to docker logs
*/
func initiateLogger() {
	// As per https://blog.sandipb.net/2018/05/04/using-zap-working-with-global-loggers/
	// Using a single global logger is discouraged, but it's a tradeoff I'm willing to make
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
}

// Accesses a number of variables in order to crash early
// if some configuration flaw exists
// We only call methods that aren't already called directly in main()
func runEnvironmentTests() {
	telegram_connector.GetTelegramToken()
	GetMensaLocationSlice()
	telegram_connector.GetReplyKeyboard()
	utils.GetLocalLocation()
	db_connectors.GetChangelogByNumber(0)
}

func initDatabases() {

	db_handle := db_connectors.GetDBHandle()
	driver, err := sqlite3.WithInstance(db_handle, &sqlite3.Config{})
	if err != nil {
		zap.S().Panic("Can't get DB driver for migrations:", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://./db/migrations", "sqlite3", driver)
	if err != nil {
		zap.S().Panic("Can't get migrate instance: ", err)
	}
	version, _, err := m.Version()
	if err != nil {
		zap.S().Panic("Can't get DB version! ", err)
	}
	if version < db_connectors.GetDBVersion() {
		err = m.Migrate(db_connectors.GetDBVersion())
		if err != nil {
			zap.S().Panic("Can't run migrations: ", err)
		}
	}

	// We also init rod, which makes sure that the
	// browser interaction works
	u := launcher.New().Bin("/usr/bin/google-chrome").MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	browser.MustPage("https://google.com").MustWaitLoad()
	browser.MustClose()
}

func main() {
	initiateLogger()
	runEnvironmentTests()
	zap.S().Info("Initializing Server...")

	// Only used for non-critical operations
	rand.Seed(time.Now().UnixNano())
	initDatabases()
	personalToken := utils.GetPersonalToken()

	mensa_scraper.ScheduleScrapeJob()
	mensa_scraper.ScheduleDailyInitialMessageJob()

	r := gin.Default()
	// r.SetTrustedProxies([]string{"172.21.0.2"})
	// We trust all proxies, [as is insecure default in gin](https://pkg.go.dev/github.com/gin-gonic/gin#readme-don-t-trust-all-proxies)
	// That shouldn't be a problem since we have
	// a reverse proxy in front of this server, and it "shouldn't" be
	// directly reachable from anywhere else.
	// We don't want to trust that reverse proxy explicitly because
	// it's wihtin our docker network, and assigning static IP addresses
	// to containers [may not be recommended](https://stackoverflow.com/questions/39493490/provide-static-ip-to-docker-containers-via-docker-compose)

	personalUrlPath := "/" + personalToken + "/"
	zap.S().Infof("Sub-URL is %s", personalUrlPath)

	r.POST(personalUrlPath, reactToRequest)
	r.Run()
}
