package mensa_scraper

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/utils"
	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
)

const MENSA_URL string = "https://xml.stw-potsdam.de/xmldata/gs/xml.php"

// Structs for representing the menu in xml
type OfferInformation struct {
	XMLName     xml.Name `xml:"angebotnr"`
	Index       string   `xml:"index,attr"`
	Title       string   `xml:"titel"`
	Description string   `xml:"beschreibung"`
}

type DateInformation struct {
	XMLName xml.Name           `xml:"datum"`
	Index   string             `xml:"index,attr"` // Formatted day, in 02.01.2006
	Offers  []OfferInformation `xml:"angebotnr"`
}

type MenuRoot struct {
	XMLName xml.Name          `xml:"menu"`
	Dates   []DateInformation `xml:"datum"`
}

func ScheduleScrapeJob() {
	schedulerInMensaTimezone := gocron.NewScheduler(utils.GetLocalLocation())
	cronBaseSyntax := "*/10 %d-%d * * 1-5" // Run every 10 minutes, every weekday, between two timestamps
	// which should be filled in from mensa opening and closing time

	mensaOpeningHours := utils.GetMensaOpeningTime().Hour()
	mensaClosingHours := utils.GetMensaClosingTime().Hour()
	formattedCronString := fmt.Sprintf(cronBaseSyntax, mensaOpeningHours, mensaClosingHours)

	if utils.IsInDebugMode() {
		formattedCronString = "*/1 * * * *"
	}
	schedulerInMensaTimezone.Cron(formattedCronString).Do(ScrapeAndAdviseUsers)

	schedulerInMensaTimezone.StartAsync()
	// Don't need to care about shutdown, shutdown happens when the container shuts down,
	// and startup happens when the container starts up

}

func ScrapeAndAdviseUsers() {
	zap.S().Info("Running mensa scrape job")
	shouldUsersBeNotified := scrapeAndInsertIfMensaMenuIsOld()
	if shouldUsersBeNotified {
		err := SendLatestMenuToUsersCurrentlyListening()
		if err != nil {
			zap.S().Error("Couldn't send menu to interested users", err)
		}
	}
}

// Returns true if something was inserted
func scrapeAndInsertIfMensaMenuIsOld() bool {
	menu, err := getMensaMenuFromWeb()
	if err != nil {
		zap.S().Errorf("Can't get menu from interweb", err)
		return false
	}
	today := time.Now().In(utils.GetLocalLocation())
	todaysInformation, err := getDateByDay(menu, today)
	if err != nil {
		zap.S().Errorf("Can't find menu for today in MenuRoot", err)
		return false
	}
	if isDateInformationFresh(todaysInformation) {
		zap.S().Debug("Mensa menu is stale")
		// No changes in menu, nothing to insert or do.
		return false
	}
	insertDateOffersIntoDBWithFreshCounter(today, todaysInformation)
	zap.S().Debug("Succesfully inserted new menu into DB")
	return true
}

func insertDateOffersIntoDBWithFreshCounter(scrapeTimestamp time.Time, dateInformation DateInformation) {

	counterValue, err := db_connectors.GetMensaMenuCounter()
	if err != nil {
		zap.S().Error("Couldn't insert new menus: Inable to get counter value", err)
		return
	}

	for _, downOffer := range dateInformation.Offers {
		offerToInsert := new(db_connectors.DBOfferInformation)
		offerToInsert.Counter = counterValue + 1
		offerToInsert.Title = downOffer.Title
		offerToInsert.Description = downOffer.Description
		offerToInsert.Time = scrapeTimestamp

		// Few enough that not batching is fine, I think
		// But batching this is something we could do
		db_connectors.InsertMensaMenu(offerToInsert)
	}
}

// Return true if the offers within this date information are the same as the
// ones we last stored in the DB
func isDateInformationFresh(dateInformation DateInformation) bool {
	// QUery DB for latest menus
	dbOffers, err := db_connectors.GetLatestMensaOffersFromToday()
	if err != nil {
		zap.S().Errorf("Can not determine freshness of queried menu, defaulting to don't insert", err)
		return true

	}
	downloadedOffers := dateInformation.Offers
	if len(downloadedOffers) != len(dbOffers) {
		return false
	}

	// Needs to be full of true
	var comparisonResultsSlice = make([]bool, len(downloadedOffers))

	// This has a runtime of n^2, but for like five elements.
	// We have duplicate title keys, so it's either that or a bunch of logic
	// that is more complicated than necessary.
	for downIndex, downOffer := range downloadedOffers {
		for dbIndex, dbOffer := range dbOffers {
			if dbOffer.Title == downOffer.Title &&
				dbOffer.Description == downOffer.Description {
				dbDayString := dbOffer.Time.Format("02.01.2006")
				if dbDayString == dateInformation.Index {
					// Elements are the same, including dates.
					// Mark this by marking true/deleting element.
					// We don't want to delete elements of the array
					// we are currently iterating, thus the true.
					// We also don't want a db element that doesn't have a download
					// partner, thus the deletion
					// Yes, this is not elegant.
					comparisonResultsSlice[downIndex] = true
					dbOffers = append(dbOffers[:dbIndex], dbOffers[dbIndex+1:]...)
				}
			}
		}
	}
	if len(dbOffers) != 0 {
		return false
	}
	for _, hasPartner := range comparisonResultsSlice {
		if !hasPartner {
			return false
		}
	}
	return true
}

func parseXML(body []byte) (MenuRoot, error) {
	// This wants to be its own function so we can
	// tets that the unmarshalling works well.
	menu := MenuRoot{}
	err := xml.Unmarshal(body, &menu)
	return menu, err
}

func getMensaMenuFromWeb() (MenuRoot, error) {
	response, err := http.Get(MENSA_URL)
	if err != nil {
		zap.S().Warn("Can't reach mensa XML. Is their service down?", err)
		return MenuRoot{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		zap.S().Warn("Can't read mensa xml response body. Is their service working?", err)
		return MenuRoot{}, err
	}
	menu, err := parseXML(body)
	if err != nil {
		zap.S().Error("Can't parse mensa xml. Did the format change?", err)
		return MenuRoot{}, err
	}
	return menu, nil
}

func getDateByDay(menu MenuRoot, day time.Time) (DateInformation, error) {
	todayString := day.Format("02.01.2006")

	for _, date := range menu.Dates {
		dayInformation := date.Index
		if dayInformation == todayString {
			return date, nil
		}
	}

	return DateInformation{}, errors.New("Can't get DateInformation for today from menu")
}
