package mensa_scraper

import (
	"fmt"
	"time"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"github.com/ADimeo/MensaQueueBot/utils"
	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
)

type MensaPreferenceSettings struct {
	ReportMon bool   `json:"reportMon"`
	ReportTue bool   `json:"reportTue"`
	ReportWed bool   `json:"reportWed"`
	ReportThu bool   `json:"reportThu"`
	ReportFri bool   `json:"reportFri"`
	FromTime  string `json:"fromTime"`
	ToTime    string `json:"toTime"`
}

var globalLastInitialMessageUTCMinute int

func ScheduleDailyInitialMessageJob() {
	nowInUTC := time.Now().UTC()
	nowUTCMinute := nowInUTC.Hour()*60 + nowInUTC.Minute()
	globalLastInitialMessageUTCMinute = nowUTCMinute
	err := scheduleNextInitialMessage(nowInUTC, nowUTCMinute)
	if err != nil {
		zap.S().Error("Couldn't' initialise initial messages", err)
	}
}

func initialMessageJob() {
	nowInUTC := time.Now().UTC()
	nowUTCMinute := nowInUTC.Hour()*60 + nowInUTC.Minute()
	if err := sendInitialMessagesThatShouldBeSentAt(nowInUTC, nowUTCMinute); err != nil {
		zap.S().Error("Couldn't' send initial messages", err)
	}
	globalLastInitialMessageUTCMinute = nowUTCMinute
	err := scheduleNextInitialMessage(nowInUTC, nowUTCMinute)
	if err != nil {
		zap.S().Error("Couldn't' schedule next initial messages", err)
	}
}

func scheduleNextInitialMessage(nowInUTC time.Time, nowUTCMinute int) error {
	utcMinuteForNextJob, err := db_connectors.GetUTCMinuteForNextIntroMessage(nowInUTC, nowUTCMinute)
	if err != nil {
		zap.S().Error("Couldn't get next time for initial messages", err)
		return err
	}
	utcHoursForJob := utcMinuteForNextJob / 60
	utcMinutesForJob := utcMinuteForNextJob % 60
	timestampString, err := fmt.Printf("%02d:%02d", utcHoursForJob, utcMinutesForJob)
	if err != nil {
		zap.S().Error("Can't create time string for next initial message mensa job", err)
	}

	schedulerInMensaTimezone := gocron.NewScheduler(utils.GetLocalLocation())
	schedulerInMensaTimezone.Every(1).Day().At(timestampString).LimitRunsTo(1).Do(initialMessageJob)

	schedulerInMensaTimezone.StartAsync()
	return nil
}

func sendInitialMessagesThatShouldBeSentAt(nowInUTC time.Time, nowUTCMinute int) error {
	users, err := db_connectors.GetUsersWithInitialMessageInTimeframe(nowInUTC, globalLastInitialMessageUTCMinute, nowUTCMinute)
	if err != nil {
		zap.S().Errorw("Can't get users that want initial message in this timeframe", "window lower bound", globalLastInitialMessageUTCMinute, "window upper bound", nowUTCMinute, "error", err)
		return err
	}

	err = sendLatestMenuToUsers(users)
	if err != nil {
		return err
	}
	return nil
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

func sendLatestMenuToUsers(idsOfInterestedUsers []int) error {
	latestOffersInDB, err := db_connectors.GetLatestMensaOffers()
	if err != nil {
		return err
	}
	formattedMessage := buildMessageFrom(latestOffersInDB)

	for _, userID := range idsOfInterestedUsers {
		telegram_connector.SendMessage(userID, formattedMessage)
	}
	return nil
}

func buildMessageFrom(offerSlice []db_connectors.DBOfferInformation) string {
	if len(offerSlice) == 0 {
		return "Griebnitzsee updated:\n\nMensa currently offers no menus"
	}

	baseMessage := "<b>Mensa Griebnitzsee Updated:</b>\n"
	baseForSingleOffer := "<i>%s:</i> %s\n"

	actualMessage := "" + baseMessage

	for _, offer := range offerSlice {
		actualMessage = actualMessage + fmt.Sprintf(baseForSingleOffer, offer.Title, offer.Description)
	}
	return actualMessage
}
