package mensa_scraper

import (
	"fmt"
	"time"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
)

func Init() {
	// Each user has a start time and an end time, a "currently receiving" flag, a "reported today" flag. (And also a "wants_mensa_messages" flag (that later will tell us which mensas))

	// Each day at mensa close remove all "reported today" flags

	// Opt in/Out code:
	// Minheap-y.
	// Check "last run at x" timestamp
	// For all opt-ins without "currently receiving" and "reported today" flag between last timestamp and now timestamp:
	//	Set the "currently receiving" flag
	// 	And send the current menu
	// For all opt-outs within last timestamp and now timetamp:
	//	Unset "currently receiving " flag
	//	Unset "reported today" flag
	// [OPT] schedule for the closest start/stop timestamp?
	// (Otherwise just run ~every 10 minutes)

	// Messaging code:
	// Every 10 minutes (or just after the menu update?)
	// Check if newest menu is different from older one
	// If it is:
	//	Get all users with "currently receiving" and without "reported today" flag
	//	Send them the updated menu

}

func SendLatestMenuToInterestedUsers() error {
	nowInUTC := time.Now().UTC()
	idsOfInterestedUsers, err := db_connectors.GetUsersToSendMenuToByTimestamp(nowInUTC)
	if err != nil {
		return err
	}

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

	baseMessage := "<b>Mensa Griebnitzsee Updated:\n</b>"
	baseForSingleOffer := "<i>%s:</i> %s\n"

	actualMessage := "" + baseMessage

	for _, offer := range offerSlice {
		actualMessage = actualMessage + fmt.Sprintf(baseForSingleOffer, offer.Title, offer.Description)
	}
	return actualMessage
}
