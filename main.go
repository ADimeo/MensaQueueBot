package main

import (
	"io/ioutil"
	"math/rand"
	"os"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"go.uber.org/zap"
)

const KEY_DEBUG_MODE string = "MENSA_QUEUE_BOT_DEBUG_MODE"

const KEY_PERSONAL_TOKEN string = "MENSA_QUEUE_BOT_PERSONAL_TOKEN"

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

func parseRequest(c *gin.Context) (*WebhookRequestBody, error) {
	body := &WebhookRequestBody{}
	err := c.ShouldBind(&body)
	return body, err
}

func sendChangelogIfNecessary(chatID int) {
	numberOfLastSentChangelog := GetLatestChangelogSentToUser(chatID)
	changelog, noChangelogWithIDError := GetChangelogByNumber(numberOfLastSentChangelog + 1)

	if noChangelogWithIDError == nil {
		sendError := SendMessage(chatID, changelog)
		if sendError == nil {
			SaveNewChangelogForUser(chatID, numberOfLastSentChangelog+1)
		} else {
			zap.S().Error("Got an error while sending changelog to user.", sendError)
		}
	}
}

func sendQueueLengthExamples(chatID int) {
	mensaLocationArray := *GetMensaLocationSlice()
	for _, mensaLocation := range mensaLocationArray {
		if mensaLocation.PhotoUrl != "" {
			err := SendStaticWebPhoto(chatID, mensaLocation.PhotoUrl, mensaLocation.Description)
			if err != nil {
				zap.S().Error("Error while sending help message photographs.", err)
			}

		}
	}
	SendTopViewOfMensa(chatID)
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
	lengthReportRegex := regexp.MustCompile(REPORT_REGEX)
	pointsRegex := regexp.MustCompile(POINTS_REGEX)

	sentMessage := bodyAsStruct.Message.Text
	chatID := bodyAsStruct.Message.Chat.ID

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
	case sentMessage == "/platypus":
		{
			url := "https://upload.wikimedia.org/wikipedia/commons/4/4a/%22Nam_Sang_Woo_Safety_Matches%22_platypus_matchbox_label_art_-_from%2C_Collectie_NMvWereldculturen%2C_TM-6477-76%2C_Etiketten_van_luciferdoosjes%2C_1900-1949_%28cropped%29.jpg"
			SendStaticWebPhoto(chatID, url, "So cute ❤️")
		}
	default:
		{
			zap.S().Infof("Received unknown message: %s", sentMessage)
		}
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
	GetTelegramToken()
	GetMensaLocationSlice()
	GetReplyKeyboard()
	GetLocalLocation()
	GetChangelogByNumber(0)
}

func initDatabases() {
	InitNewDB()
	InitNewChangelogDB()
	InitNewInternetPointsDB()
	// We also init rod, which makes sure that the
	// browser interaction works
	u := launcher.New().Bin("/usr/bin/google-chrome").MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	browser.MustPage("https://google.com").MustWaitLoad()
	browser.MustClose()
}

/*IsInDebugMode can be used to change behaviour
for testing. Currently mostly used to allow
reports at weird times
*/
func IsInDebugMode() bool {
	_, doesExist := os.LookupEnv(KEY_DEBUG_MODE)
	if !doesExist {
		return false
	}
	return true
}

func main() {
	initiateLogger()
	runEnvironmentTests()
	zap.S().Info("Initializing Server...")

	// Only used for non-critical operations
	rand.Seed(time.Now().UnixNano())
	initDatabases()
	personalToken := GetPersonalToken()

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
