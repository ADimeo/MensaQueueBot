package mensa_scraper

import (
	"fmt"
	"strconv"
	"time"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"github.com/ADimeo/MensaQueueBot/utils"
	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
)

type MensaPreferenceSettings struct {
	ReportAtAll bool   `json:"reportAtall"`
	ReportMon   bool   `json:"reportMon"`
	ReportTue   bool   `json:"reportTue"`
	ReportWed   bool   `json:"reportWed"`
	ReportThu   bool   `json:"reportThu"`
	ReportFri   bool   `json:"reportFri"`
	FromTime    string `json:"fromTime"`
	ToTime      string `json:"toTime"`
}

func (settingsStruct MensaPreferenceSettings) GetWeekdayBitmap() int {
	bitmap := 0
	// rightmost bit (position 0) is saturday, is 0
	// leftmost bit (position 6) is sunday, is 0
	if settingsStruct.ReportFri {
		// Is there really no prettier way to do this?
		bitmap += 1 << 1
	}
	if settingsStruct.ReportThu {
		bitmap += 1 << 2
	}
	if settingsStruct.ReportWed {
		bitmap += 1 << 3
	}
	if settingsStruct.ReportTue {
		bitmap += 1 << 4
	}
	if settingsStruct.ReportMon {
		bitmap += 1 << 5
	}
	return bitmap
}

func (settingsStruct MensaPreferenceSettings) GetFromTimeAsCESTMinute() (int, error) {
	// We expect a format like 12:00
	hour, err := strconv.Atoi(settingsStruct.FromTime[0:2])
	if err != nil {
		zap.S().Errorw("Can't convert FromTime string to actual int", "FromTime value", settingsStruct.FromTime, "error", err)
		return 600, err
	}
	minute, err := strconv.Atoi(settingsStruct.FromTime[3:5])
	if err != nil {
		zap.S().Errorw("Can't convert FromTime string to actual int", "FromTime value", settingsStruct.FromTime, "error", err)
		return 600, err
	}

	cestMinute := hour*60 + minute
	if err != nil {
		zap.S().Errorw("Can't convert FromTime string to actual int", "FromTime value", settingsStruct.FromTime, "error", err)
		return 600, err
	}
	return cestMinute, nil
}

func (settingsStruct *MensaPreferenceSettings) GetToTimeAsCESTMinute() (int, error) {
	// We expect a format like 12:00
	hour, err := strconv.Atoi(settingsStruct.ToTime[0:2])
	if err != nil {
		zap.S().Errorw("Can't convert ToTime string to actual int", "FromTime value", settingsStruct.ToTime, "error", err)
		return 840, err
	}

	minute, err := strconv.Atoi(settingsStruct.ToTime[3:5])
	if err != nil {
		zap.S().Errorw("Can't convert ToTime string to actual int", "FromTime value", settingsStruct.ToTime, "error", err)
		return 840, err
	}

	cestMinute := hour*60 + minute
	if err != nil {
		zap.S().Errorw("Can't convert ToTime string to actual int", "FromTime value", settingsStruct.ToTime, "error", err)
		return 840, err
	}
	return cestMinute, nil
}

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
