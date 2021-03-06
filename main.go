package main

import (
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

/*
   Sends a thank you message for a report

*/
func sendThankYouMessage(chatID int, textSentByUser string) {
	emojiRune := getRandomAcceptableEmoji()
	baseMessage := "You reported length %s, thanks " + string(emojiRune)

	zap.S().Infof("Sending thank you for %s", textSentByUser)

	err := SendMessage(chatID, fmt.Sprintf(baseMessage, textSentByUser))
	if err != nil {
		zap.S().Error("Error while sending thank you message.", err)
	}
}

func sendNoThanksMessage(chatID int, textSentByUser string) {
	emojiRune := getRandomAcceptableEmoji()
	baseMessage := "...are you sure?" + string(emojiRune)

	zap.S().Infof("Sending no thanks for %s", textSentByUser)

	err := SendMessage(chatID, baseMessage)
	if err != nil {
		zap.S().Error("Error while sending no thanks message.", err)
	}
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

	potsdamLocation := GetLocalLocation()
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
	mensaLocationArray := *GetMensaLocationSlice()
	for _, mensaLocation := range mensaLocationArray {
		err := SendPhoto(chatID, mensaLocation.PhotoUrl, mensaLocation.Description)
		if err != nil {
			zap.S().Error("Error while sending help message photographs.", err)
		}
	}
	SendTopViewOfMensa(chatID)
}

func sendPointsRequestedResponse(chatID int, currentlyOptedIn bool, points int) error {
	emojiRune := getRandomAcceptableEmoji()
	baseMessage := "You have collected %d points%s" + string(emojiRune)
	var encouragements = [...]string{
		", that's a good start ????",
		", which is like two weeks of reporting every singe day ???????",
		", way to go! ????",
		". You can officially claim that you're a professional mensa queue length reporter, and I'll support that claim. ????",
		". Consider me impressed ????",
		". Do you always go above and beyond? ????",
		". Wow. ????",
		", and I'll be honest, I don't know what to say ????",
	}

	explanationMessage := `You're currently not collecting points, but please know that we greatly appreciate all reports. For information about points send /points_help`

	var err error
	zap.S().Info("Sending pointsrequest message.")
	if currentlyOptedIn {
		pointsCollected := GetNumberOfPointsByUser(chatID)
		encouragementSelector := pointsCollected / 9 // New encouragement message every 9 points
		if encouragementSelector >= len(encouragements) {
			encouragementSelector = len(encouragements) - 1
		}

		encouragementMessage := encouragements[encouragementSelector]
		messageToSend := fmt.Sprintf(baseMessage, pointsCollected, encouragementMessage)
		err := SendMessage(chatID, messageToSend)
		if err != nil {
			zap.S().Errorf("Error while sending pointsrequest message for %s points", points, err)
		}
	} else {
		err = SendMessage(chatID, explanationMessage)
		if err != nil {
			zap.S().Error("Error while sending pointsrequest message.", err)
		}
	}
	return err
}

func sendPointsOptInResponse(chatID int, currentlyOptedIn bool) {
	messageOptIn := "Alrighty, from now on you're collecting points ????"
	messageDoubleOptIn := "Sure, but you were already collecting points ????"

	var err error
	if currentlyOptedIn {
		err = SendMessage(chatID, messageDoubleOptIn)
	} else {
		err = SendMessage(chatID, messageOptIn)
	}
	if err != nil {
		zap.S().Error("Error while sending points opt-in message.", err)
	}
}

func sendPointsOptOutResponse(chatID int, currentlyOptedIn bool) {
	messageOptOut := "You're the boss, all your points have been deleted ????"
	messageDoubleOptOut := "There's nothing to delete: You weren't collecting points ????"

	var err error
	if currentlyOptedIn {
		err = SendMessage(chatID, messageOptOut)
	} else {
		err = SendMessage(chatID, messageDoubleOptOut)
	}
	if err != nil {
		zap.S().Error("Error while sending points opt-out message.", err)
	}
}

func sendPointsHelpMessages(chatID int) {
	var messageArray = [...]string{
		"If you want to, you can opt in to collect internetpoints for your reports!",
		"You get one point for each report, and your points will add up with each report you make",
		"Here at MensaQueueBot, we try to minimize the data we collect. Right now all your reports are anonymized. Your reports will stay anonymous regardless of whether you collect points or not, but if you opt in we'll need to store additional information, specifically how many reports you've made. Just wanted to let you know that.",
		"Right now points don't do anything except prove to everybody what a great reporter you are, but we have plans for the future! (Maybe!)",
		`To start collecting points send /points_track`,
		`To stop colletcing points and delete all data related to point collection send /points_delete`,
		`To see your points send /points`,
	}
	for i := 0; i < len(messageArray); i++ {
		messageString := messageArray[i]
		err := SendMessage(chatID, messageString)
		if err != nil {
			zap.S().Error("Error while sending help message for point", err)
		}
	}
}

func reportAppearsValid(reportText string) bool {
	// Checking time: It's not on the weekend
	var today = time.Now()

	if today.Weekday() == 0 || today.Weekday() == 6 {
		// Sunday or Saturday, per https://golang.google.cn/pkg/time/#Weekday
		zap.S().Info("Report is on weekend")
		return false
	}

	if GetMensaOpeningTime().After(today) ||
		GetMensaClosingTime().Before(today) {
		zap.S().Info("Report is outside of mensa hours")
		// Outside of mensa closing times
		return false
	}
	zap.S().Info("Report is considered valid")
	return true

}

func handlePointsRequest(sentMessage string, chatID int) {
	userIsCollectingPoints := UserIsCollectingPoints(chatID)

	if sentMessage == "/points" {
		points := 0
		if userIsCollectingPoints {
			points = GetNumberOfPointsByUser(chatID)
		}
		sendPointsRequestedResponse(chatID, userIsCollectingPoints, points)
	} else if sentMessage == "/points_track" {
		if userIsCollectingPoints {
			// Nothing to do: User is already opted in
		} else {
			EnableCollectionOfPoints(chatID)
		}
		sendPointsOptInResponse(chatID, userIsCollectingPoints)
	} else if sentMessage == "/points_delete" {
		if userIsCollectingPoints {
			DisableCollectionOfPoints(chatID)
		} else {
			// Nothing to do: User is already opted out
		}
		sendPointsOptOutResponse(chatID, userIsCollectingPoints)
	} else if sentMessage == "/points_help" {
		sendPointsHelpMessages(chatID)
	} else {
		zap.S().Infof("Usermessage '%s' does not match with any point message", sentMessage)
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
			handlePointsRequest(sentMessage, chatID)
			sendChangelogIfNecessary(chatID)
		}
	case sentMessage == "/jetze":
		{
			zap.S().Infof("Received a /jetze request")
			time, reportedQueueLength := GetLatestQueueLengthReport()
			sendQueueLengthReport(chatID, time, reportedQueueLength)
			sendChangelogIfNecessary(chatID)
		}
	case sentMessage == "/jetze@MensaQueueBot":
		zap.S().Infof("Received a /jetze request, but in a group")
		time, reportedQueueLength := GetLatestQueueLengthReport()
		sendQueueLengthReport(chatID, time, reportedQueueLength)
		sendChangelogIfNecessary(chatID)
	case lengthReportRegex.Match([]byte(sentMessage)):
		{
			zap.S().Infof("Received a new report: %s", sentMessage)
			if reportAppearsValid(sentMessage) {
				messageUnixTime := bodyAsStruct.Message.Date
				errorWhileSaving := saveQueueLength(sentMessage, messageUnixTime, chatID)
				if errorWhileSaving == nil {
					if UserIsCollectingPoints(chatID) {
						AddInternetPoint(chatID)
					}
					sendThankYouMessage(chatID, sentMessage)
				}
			} else {
				sendNoThanksMessage(chatID, sentMessage)
			}
			sendChangelogIfNecessary(chatID)
		}
	case sentMessage == "/platypus":
		{
			url := "https://upload.wikimedia.org/wikipedia/commons/4/4a/%22Nam_Sang_Woo_Safety_Matches%22_platypus_matchbox_label_art_-_from%2C_Collectie_NMvWereldculturen%2C_TM-6477-76%2C_Etiketten_van_luciferdoosjes%2C_1900-1949_%28cropped%29.jpg"
			SendPhoto(chatID, url, "So cute ??????")
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
