package mensa_scraper

import (
	"errors"
	"fmt"
	"time"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"github.com/ADimeo/MensaQueueBot/utils"
	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
)

var globalLastInitialMessageCESTMinute int

func ScheduleDailyInitialMessageJob() {
	nowInUTC := time.Now().UTC()
	nowInLocal := nowInUTC.In(utils.GetLocalLocation())

	nowCESTMinute := nowInLocal.Hour()*60 + nowInLocal.Minute()
	globalLastInitialMessageCESTMinute = nowCESTMinute
	err := scheduleNextInitialMessage(nowInUTC, nowCESTMinute)
	if err != nil {
		zap.S().Error("Couldn't' initialise initial messages", err)
	}
}

func initialMessageJob() {
	nowInUTC := time.Now().UTC()
	nowInLocal := nowInUTC.In(utils.GetLocalLocation())
	nowCESTMinute := nowInLocal.Hour()*60 + nowInLocal.Minute()
	if err := sendInitialMessagesThatShouldBeSentAt(nowInUTC, nowCESTMinute); err != nil {
		zap.S().Error("Couldn't' send initial messages", err)
	}
	globalLastInitialMessageCESTMinute = nowCESTMinute
	err := scheduleNextInitialMessage(nowInUTC, nowCESTMinute)
	if err != nil {
		zap.S().Error("Couldn't' schedule next initial messages", err)
	}
}

func scheduleNextInitialMessage(nowInUTC time.Time, nowCESTMinute int) error {
	cestMinuteForNextJob, err := db_connectors.GetCESTMinuteForNextIntroMessage(nowInUTC, nowCESTMinute)
	if err != nil {
		zap.S().Error("Couldn't get next time for initial messages", err)
		return err
	}
	cestHoursForJob := cestMinuteForNextJob / 60
	cestMinutesForJob := cestMinuteForNextJob % 60
	timestampString, err := fmt.Printf("%02d:%02d", cestHoursForJob, cestMinutesForJob)
	if err != nil {
		zap.S().Error("Can't create time string for next initial message mensa job", err)
	}

	schedulerInMensaTimezone := gocron.NewScheduler(utils.GetLocalLocation())
	schedulerInMensaTimezone.Every(1).Day().At(timestampString).LimitRunsTo(1).Do(initialMessageJob)

	schedulerInMensaTimezone.StartAsync()
	return nil
}

func sendInitialMessagesThatShouldBeSentAt(nowInUTC time.Time, nowCESTMinute int) error {
	users, err := db_connectors.GetUsersWithInitialMessageInTimeframe(nowInUTC, globalLastInitialMessageCESTMinute, nowCESTMinute)
	if err != nil {
		zap.S().Errorw("Can't get users that want initial message in this timeframe", "window lower bound", globalLastInitialMessageCESTMinute, "window upper bound", nowCESTMinute, "error", err)
		return err
	}

	err = sendLatestMenuToUsers(users)
	return err
}

func SendLatestMenuToUsersCurrentlyListening() error {
	// Called by menu scraper
	nowInUTC := time.Now().UTC()
	idsOfInterestedUsers, err := db_connectors.GetUsersToSendMenuToByTimestamp(nowInUTC)
	if err != nil {
		return err
	}
	return sendLatestMenuToUsers(idsOfInterestedUsers)
}

func SendLatestMenuToSingleUser(userID int) error {
	sliceOfUserID := []int{userID}

	return sendLatestMenuToUsers(sliceOfUserID)
}

func sendLatestMenuToUsers(idsOfInterestedUsers []int) error {
	latestOffersInDB, err := db_connectors.GetLatestMensaOffersFromToday()
	if err != nil {
		return err
	}
	if len(latestOffersInDB) == 0 {
		return errors.New("No menu from today available")

	}
	formattedMessage := buildMessageFrom(latestOffersInDB)

	for _, userID := range idsOfInterestedUsers {
		keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.PUSH_MESSAGE, userID)
		telegram_connector.SendMessage(userID, formattedMessage, keyboardIdentifier)
	}
	return nil
}

func buildMessageFrom(offerSlice []db_connectors.DBOfferInformation) string {
	if len(offerSlice) == 0 {
		return "Griebnitzsee currently offers no menus"
	}

	baseMessage := "<b>Current Griebnitzsee Menu:</b>\n"
	baseForSingleOffer := "<i>%s:</i> %s\n"

	actualMessage := "" + baseMessage

	for _, offer := range offerSlice {
		actualMessage = actualMessage + fmt.Sprintf(baseForSingleOffer, offer.Title, offer.Description)
	}
	return actualMessage
}
