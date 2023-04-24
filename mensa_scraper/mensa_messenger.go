package mensa_scraper

import (
	"errors"
	"fmt"
	"time"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"github.com/ADimeo/MensaQueueBot/utils"
	"github.com/go-co-op/gocron"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"
)

var globalLastInitialMessageCESTMinute int

// Please only modify in scheduleNextInitialMessage
// Gets overridden regularly
var globalInitialMessageScheduler *gocron.Scheduler

/*
Responsible for initialising first of two regular jobs.
"DailyInitialMessageJob" sends the initial message to users,
which is to say the message a user receives when their window of interest
for mensa menus opens (sent at 09:00 if user wants info from 09:00 to 10:00).

This task schedules itself hop-to-hop, so on each execution it queries the DB
for when it should next run, and schedules a new task for that point in time.

*/
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
	zap.S().Infof("Prepping to send initial messages...")
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

/*
For a given timeStringInCEST this updates the next initialMessage job.
Specifically, if we would skip the newly inserted initial message
(Since it would still run today, but the next scheduled job is after
this message should run) we clear the initial job, and recreate it
with this given message in mind.

Call after DB operation of inserting the new settings have finished!
This queries the DB

This function will reschedule overeagerly, but that shouldn't be a problem
since underlying functions should be idempotent if run at the same time with
the same db state
*/
func RescheduleNextInitialMessageJobIfNeeded(insertedTimeStringInCEST string) {
	if globalInitialMessageScheduler.Len() == 0 {
		// Scheduler has no jobs at all
		// This should not happen, but let's schedule a job to fix it
		zap.S().Error("Initial job scheduler had no jobs during settings change")
		nowInUTC := time.Now().UTC()
		nowInCEST := nowInUTC.In(utils.GetLocalLocation())
		nowCESTMinute := nowInCEST.Hour()*60 + nowInCEST.Minute()
		scheduleNextInitialMessage(nowInUTC, nowCESTMinute)
		return
	}

	nextInitialJob := globalInitialMessageScheduler.Jobs()[0]
	jobTimeInCEST := nextInitialJob.ScheduledAtTime() // Returns 10:00 string
	jobTimeAsTime, _ := time.Parse("15:04", jobTimeInCEST)
	newTimeAsTime, _ := time.Parse("15:04", insertedTimeStringInCEST)
	if newTimeAsTime.Before(jobTimeAsTime) {
		// Newly scheduled job would be up first.
		// Q: Will it need to be scheduled for today?
		// (Technically scheduleNextInitialMessage should take care of that case,
		// but this adds some redundancy
		nowInUTC := time.Now().UTC()
		nowInCEST := nowInUTC.In(utils.GetLocalLocation())
		// Let's add two minutes of leeway, just so we don't accidentally schedule this for tomorrow
		twoMinutes, _ := time.ParseDuration("2m")
		soonInCEST := nowInCEST.Add(twoMinutes)
		soonTimeStringInCEST := soonInCEST.Format("15:04")
		soonTime, _ := time.Parse("15:04", soonTimeStringInCEST)
		if newTimeAsTime.After(soonTime) {
			zap.S().Info("Rescheduling initial menu job during settings change")
			// This is would be the next job for today,
			// We need to reschedule
			globalInitialMessageScheduler.Clear()
			nowCESTMinute := nowInCEST.Hour()*60 + nowInCEST.Minute()
			scheduleNextInitialMessage(nowInUTC, nowCESTMinute)
		}
	}
}

/*
scheduleNextInitialMessage schedules the next initialMessage job based on what is stored in the DB.
Specifically, this will return the next time during which any user wants to
receive a mensa menu update which
- is after nowCESTMinute (hh*60+mm)
- user wants messages
- on this weekday
- and hasn't reported yet

*/
func scheduleNextInitialMessage(nowInUTC time.Time, nowCESTMinute int) error {
	cestMinuteForNextJob, err := db_connectors.GetCESTMinuteForNextIntroMessage(nowInUTC, nowCESTMinute)
	if err != nil {
		zap.S().Error("Couldn't get next time for initial messages", err)
		return err
	}
	cestHoursForJob := cestMinuteForNextJob / 60
	cestMinutesForJob := cestMinuteForNextJob % 60
	timestampString := fmt.Sprintf("%02d:%02d", cestHoursForJob, cestMinutesForJob)

	globalInitialMessageScheduler = gocron.NewScheduler(utils.GetLocalLocation())
	globalInitialMessageScheduler.Every(1).Day().At(timestampString).LimitRunsTo(1).Do(initialMessageJob)
	zap.S().Infof("Next initial message scheduled for %s", timestampString)

	globalInitialMessageScheduler.StartAsync()
	return nil
}

func sendInitialMessagesThatShouldBeSentAt(nowInUTC time.Time, nowCESTMinute int) error {
	var openingCESTMinute int
	if globalLastInitialMessageCESTMinute > nowCESTMinute {
		// The last message we sent had a timestamp after now, this means it was send on a different day
		// -> Re-Start the interval at 0
		openingCESTMinute = 0
	} else {
		openingCESTMinute = globalLastInitialMessageCESTMinute
	}
	users, err := db_connectors.GetUsersWithInitialMessageInTimeframe(nowInUTC, openingCESTMinute, nowCESTMinute)
	if err != nil {
		zap.S().Errorw("Can't get users that want initial message in this timeframe", "window lower bound", globalLastInitialMessageCESTMinute, "window upper bound", nowCESTMinute, "error", err)
		return err
	}

	return sendLatestMenuToUsers(users)
}

/*
SendLatestMenuToUsersCurrentlyListening gets a list of all users which want to be advised about a mensa change right now,
and sends them a message of the current menu.
Via the queries this calls it keeps in mind additional requirements (weekday, send at all flag),
previously reported on this date)
*/
func SendLatestMenuToUsersCurrentlyListening() error {
	// Called by menu scraper
	nowInUTC := time.Now().UTC()
	idsOfInterestedUsers, err := db_connectors.GetUsersToSendMenuToByTimestamp(nowInUTC)
	if err != nil {
		return err
	}
	return sendLatestMenuToUsers(idsOfInterestedUsers)
}

/*
SendLatestMenuToSingleUser sends tha most recently added menu to the given user

*/
func SendLatestMenuToSingleUser(userID int) error {
	sliceOfUserID := []int{userID}

	return sendLatestMenuToUsers(sliceOfUserID)
}

func sendLatestMenuToUsers(idsOfInterestedUsers []int) error {
	if len(idsOfInterestedUsers) == 0 {
		zap.S().Infof("Tried to send latest menu to empty list of users")
		return nil
	}
	latestOffersInDB, err := db_connectors.GetLatestMensaOffersFromToday()
	if err != nil {
		return err
	}
	if len(latestOffersInDB) == 0 {
		return errors.New("No menu from today available")
	}
	formattedMessage := buildMessageFrom(latestOffersInDB)

	var errorsForAllSends error
	for _, userID := range idsOfInterestedUsers {
		keyboardIdentifier := telegram_connector.GetIdentifierViaRequestType(telegram_connector.PUSH_MESSAGE, userID)
		if err = telegram_connector.SendMessage(userID, formattedMessage, keyboardIdentifier); err != nil {
			errorsForAllSends = multierror.Append(errorsForAllSends, err)
		}
	}
	return errorsForAllSends
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
