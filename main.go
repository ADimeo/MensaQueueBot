package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const KEY_PERSONAL_TOKEN string = "MENSA_QUEUE_BOT_PERSONAL_TOKEN"

const MENSA_LOCATION_JSON_LOCATION string = "./mensa_locations.json"

const REPORT_REGEX string = `^L\d: ` // A message that matches this regex is a length report, and should be treated as such

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
   Contains a number of messages that should be sent to users as an introduction.
   Should be sent together with the image (links) defined in getMensaLocationSlice.

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
		// Send pictures here
		"Once we have collected enough data we'll provide you with an overview of when on which days the mensa queue is shortest - that means you'll waste less time just standing in line",
		"You can also use /jetze to find out what length the mensa queue has right now, but be aware that queue lengths can quickly change",
		"If you use /help the bot will send the queue length examples again",
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
func getMensaLocationSlice() *[]mensaLocation {
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

/*
   Reads the personal token from environment variables.
   The personal token is part of the url path, and tries to prevent non-authorized users from accessing our webhooks, and therefore spamming our users.
   For this purpose it needs to be long, random, and non-public.
*/
func getPersonalToken() string {
	personalKey, doesExist := os.LookupEnv(KEY_PERSONAL_TOKEN)

	if !doesExist {
		zap.S().Panicf("Fatal Error: Environment variable for personal key not set: %s", KEY_PERSONAL_TOKEN)
	}
	return personalKey
}

func parseRequest(c *gin.Context) (*WebhookRequestBody, error) {
	body := &WebhookRequestBody{}
	err := c.ShouldBind(&body)
	return body, err
}

/*
   Writes the given queue length to the database
*/
func saveQueueLength(queueLength string, unixTimestamp int, chatID int) error {
	chatIDString := strconv.Itoa(chatID)
	return WriteReportToDB(chatIDString, unixTimestamp, queueLength)
}

/*
   Sends a number of messages to the specified user, explaining the base concept and instructing them on how to act
   Tightly coupled with getWelcomeMessageArray and getMensaLocationSlice
*/
func sendWelcomeMessage(chatID int) {
	messageArray := getWelcomeMessageArray()
	mensaLocationArray := *getMensaLocationSlice()

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
	// Send all photos
	for _, mensaLocation := range mensaLocationArray {
		err = SendPhoto(chatID, mensaLocation.PhotoUrl, mensaLocation.Description)
		if err != nil {
			zap.S().Error("Error while sending queue length examples.", err)
		}
	}

	for i := 5; i < 9; i++ {
		messageString := messageArray[i]
		err = SendMessage(chatID, messageString)
		if err != nil {
			zap.S().Error("Error while sending final welcome messages.", err)
		}
	}
}

/*
   People like emoji. People also like slot machines. Return a random, pre-vetted emoji when they
   report, for "engagement"

    One emoji per day is chosen.
*/
func getRandomAcceptableEmoji() rune {
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

/*
   Sends a thank you message for a report

*/
func sendThankYouMessage(chatID int, textSentByUser string) {
	emojiRune := getRandomAcceptableEmoji()
	baseMessage := "You reported length %s, thanks" + string(emojiRune)

	// Say what they logged, and thanks

	zap.S().Infof("Sending thank you for %s", textSentByUser)

	err := SendMessage(chatID, fmt.Sprintf(baseMessage, textSentByUser))
	if err != nil {
		zap.S().Error("Error while sending thank you message.", err)
	}
}

func getLocalLocation() *time.Location {
	potsdamLocation, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		zap.S().Panic("Can't load location Europe/Berlin!")
	}
	return potsdamLocation
}

/*
   Sends a message to the specified user, depending on when the last reported queue length was;
   - For reported lengths within the last 5 minutes
   - For reported lengths within the last 59 minutes
   - For reported lengths on the same day
   - For no reported length on the same day
*/
func sendQueueLengthReport(chatID int, timeOfReport int, reportedQueueLength string) {
	baseMessageReportAvailable := "Current length of mensa queue is %s"
	baseMessageRelativeReportAvailable := "%d minutes ago the length was %s"
	baseMessageNoRecentReportAvailable := "No recent report, but today at %s the length was %s"
	baseMessageNoReportAvailable := "No queue length reported today."

	acceptableDeltaSinceLastReport, _ := time.ParseDuration("5m")
	timeDeltaForRelativeTimeSinceLastReport, _ := time.ParseDuration("59m")

	timestampNow := time.Now()
	timestampThen := time.Unix(int64(timeOfReport), 0)

	potsdamLocation := getLocalLocation()
	timestampNow = timestampNow.In(potsdamLocation)
	timestampThen = timestampThen.In(potsdamLocation)

	zap.S().Infof("Loading queue length report from %s Europe/Berlin(Current time is %s Europe/Berlin)", timestampThen.Format("15:04"), timestampNow.Format("15:04"))

	var err error

	timeSinceLastReport := timestampNow.Sub(timestampThen)
	if timeSinceLastReport <= acceptableDeltaSinceLastReport {
		err = SendMessage(chatID, fmt.Sprintf(baseMessageReportAvailable, reportedQueueLength))
	} else if timeSinceLastReport <= timeDeltaForRelativeTimeSinceLastReport {
		err = SendMessage(chatID, fmt.Sprintf(baseMessageRelativeReportAvailable, int(timeSinceLastReport.Minutes()), reportedQueueLength))
	} else if timestampNow.YearDay() == timestampThen.YearDay() {
		err = SendMessage(chatID, fmt.Sprintf(baseMessageNoRecentReportAvailable, timestampThen.Format("15:04"), reportedQueueLength))
	} else {
		err = SendMessage(chatID, baseMessageNoReportAvailable)

	}
	if err != nil {
		zap.S().Error("Error while sending queue length report", err)
	}

}

func sendQueueLengthExamples(chatID int) {
	mensaLocationArray := *getMensaLocationSlice()
	for _, mensaLocation := range mensaLocationArray {
		err := SendPhoto(chatID, mensaLocation.PhotoUrl, mensaLocation.Description)
		if err != nil {
			zap.S().Error("Error while sending help message photographs.", err)
		}
	}
}

func reactToRequest(ginContext *gin.Context) {
	// Return some 200 or something

	bodyAsStruct, err := parseRequest(ginContext)
	if err == nil {
		ginContext.JSON(200, gin.H{
			"message": "Thanks nice serverr",
		})
	} else {
		zap.S().Error("Inbound data from telegram couldn't be parsed", err)
	}
	lengthReportRegex := regexp.MustCompile(REPORT_REGEX)

	sentMessage := bodyAsStruct.Message.Text
	chatID := bodyAsStruct.Message.Chat.ID

	switch {
	case sentMessage == "/start":
		{
			zap.S().Info("Sending onboarding (/start) messages")
			sendWelcomeMessage(chatID)
		}
	case sentMessage == "/help":
		{
			zap.S().Info("Sending queue length (/help) messages")
			sendQueueLengthExamples(chatID)
		}
	case sentMessage == "/jetze":
		{
			zap.S().Infof("Received a /jetze request")
			time, reportedQueueLength := GetLatestQueueLengthReport()
			sendQueueLengthReport(chatID, time, reportedQueueLength)
		}
	case lengthReportRegex.Match([]byte(sentMessage)):
		{
			zap.S().Infof("Received a new report: %s", sentMessage)
			messageUnixTime := bodyAsStruct.Message.Date
			errorWhileSaving := saveQueueLength(sentMessage, messageUnixTime, chatID)
			if errorWhileSaving == nil {
				sendThankYouMessage(chatID, sentMessage)
			}
		}
	case sentMessage == "/platypus":
		{
			url := "https://upload.wikimedia.org/wikipedia/commons/4/4a/%22Nam_Sang_Woo_Safety_Matches%22_platypus_matchbox_label_art_-_from%2C_Collectie_NMvWereldculturen%2C_TM-6477-76%2C_Etiketten_van_luciferdoosjes%2C_1900-1949_%28cropped%29.jpg"
			SendPhoto(chatID, url, "So cute ❤️")
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
	getMensaLocationSlice()
	GetReplyKeyboard()
	getLocalLocation()
}

func main() {
	initiateLogger()
	runEnvironmentTests()
	zap.S().Info("Initializing Server...")

	// Only used for non-critical operations
	rand.Seed(time.Now().UnixNano())
	InitNewDB()
	personalToken := getPersonalToken()

	r := gin.Default()
	// r.SetTrustedProxies([]string{"172.21.0.2"})
	// We trust all proxies, [as is insecure default in gin](https://pkg.go.dev/github.com/gin-gonic/gin#readme-don-t-trust-all-proxies)
	//That shouldn't be a problem since we have
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
