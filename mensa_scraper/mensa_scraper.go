package mensa_scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/utils"
	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
)

const MENSA_URL string = "https://swp.webspeiseplan.de/index.php?token=55ed21609e26bbf68ba2b19390bf7961&model=menu&location=9601&languagetype=1&_=1696321056188" // Please be static token, pleeaaaaase!
const MENSA_TITLE_URL string = "https://swp.webspeiseplan.de/index.php?token=55ed21609e26bbf68ba2b19390bf7961&model=mealCategory&location=9601&languagetype=1&_=1696589384933"

// Structs for representing the menu/title in json
type EssensTitle struct {
	Name                string `json:"name"`
	GerichtskategorieID int    `json:"gerichtkategorieID"`
}

type EssensTitleRoot struct {
	Success bool          `json:"success"`
	Content []EssensTitle `json:"content"`
}

type SpeiseplanAdvancedGericht struct {
	Aktiv              bool   `json:"aktiv"`
	Datum              string `json:"datum"`
	GerichtKategorieID int    `json:"gerichtkategorieID"`
	Gerichtname        string `json:"gerichtname"`
	GerichtTitle       string `json:"enrichThisManually,omitempty"` // Enriched manually
}

type SpeiseplanAdvancedGerichtData struct {
	Gericht SpeiseplanAdvancedGericht `json:"SpeiseplanAdvancedGericht"`
}

// This contains two objects, one for each week - but we only need the Gericht
type SpeiseplanWeek struct {
	SpeiseplanGerichtData []SpeiseplanAdvancedGerichtData `json:"speiseplanGerichtData"`
}

type MenuRoot struct {
	Success bool             `json:"success"`
	Content []SpeiseplanWeek `json:"content"`
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
	mealsForToday := getMealsForToday(menu, today)
	if len(mealsForToday) == 0 {
		zap.S().Errorf("Can't find meals for today", err)
		return false
	}
	todaysInformationWithTitles, err := enrichWithTitleData(mealsForToday)

	if isDateInformationFresh(todaysInformationWithTitles) {
		zap.S().Debug("Mensa menu is stale")
		// No changes in menu, nothing to insert or do.
		return false
	}
	insertDateOffersIntoDBWithFreshCounter(today, todaysInformationWithTitles)
	zap.S().Debug("Succesfully inserted new menu into DB")
	return true
}

func enrichWithTitleData(todaysInformation []SpeiseplanAdvancedGericht) ([]SpeiseplanAdvancedGericht, error) {
	mensaEnrichmentClient := &http.Client{}
	mensaEnrichmentRequest, _ := http.NewRequest("GET", MENSA_TITLE_URL, nil)
	mensaEnrichmentRequest.Header.Set("Referer", "https://swp.webspeiseplan.de/Menu")

	response, err := mensaEnrichmentClient.Do(mensaEnrichmentRequest)

	if err != nil {
		zap.S().Warn("Can't reach json with meal<->Essen N mapping. Is their service down?", err)
		return todaysInformation, err
	}

	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		zap.S().Warn("Can't read json with meal<-> essen mapping. Is their service working?", err)
		return todaysInformation, err
	}

	titleRoot, err := parseTitleJSON(body)
	if err != nil {
		zap.S().Error("Can't parse title json. Did the format change?", err)
		return todaysInformation, err
	}

	for index, meal := range todaysInformation {
		mealID := meal.GerichtKategorieID

		for _, titleObject := range titleRoot.Content {
			if titleObject.GerichtskategorieID == mealID {
				meal.GerichtTitle = titleObject.Name
				todaysInformation[index] = meal
				break
			}
		}
	}
	return todaysInformation, nil
}

func insertDateOffersIntoDBWithFreshCounter(scrapeTimestamp time.Time, todaysInformationWithTitles []SpeiseplanAdvancedGericht) {
	counterValue, err := db_connectors.GetMensaMenuCounter()
	if err != nil {
		zap.S().Error("Couldn't insert new menus: Unable to get counter value", err)
		return
	}

	for _, downOffer := range todaysInformationWithTitles {
		offerToInsert := new(db_connectors.DBOfferInformation)
		offerToInsert.Counter = counterValue + 1
		offerToInsert.Title = downOffer.GerichtTitle
		offerToInsert.Description = downOffer.Gerichtname
		offerToInsert.Time = scrapeTimestamp

		// Few enough that not batching is fine, I think
		// But batching this is something we could do
		db_connectors.InsertMensaMenu(offerToInsert)
	}
}

// Return true if the offers within this date information are the same as the
// ones we last stored in the DB
func isDateInformationFresh(mealsForToday []SpeiseplanAdvancedGericht) bool {
	// Query DB for latest menus
	dbOffers, err := db_connectors.GetLatestMensaOffersFromToday()
	if err != nil {
		zap.S().Errorf("Can not determine freshness of queried menu, defaulting to don't insert", err)
		return true

	}
	if len(mealsForToday) != len(dbOffers) {
		return false
	}

	// Needs to be full of true
	var comparisonResultsSlice = make([]bool, len(mealsForToday))

	// This has a runtime of n^2, but for like five elements.
	// We have duplicate title keys, so it's either that or a bunch of logic
	// that is more complicated than necessary.
	for downIndex, downOffer := range mealsForToday {
		for dbIndex, dbOffer := range dbOffers {
			if dbOffer.Title == downOffer.GerichtTitle &&
				dbOffer.Description == downOffer.Gerichtname {
				dbDayString := dbOffer.Time.Format("2006-01-02")
				if dbDayString == downOffer.Datum {
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

func parseJSON(body []byte) (MenuRoot, error) {
	// This wants to be its own function so we can
	// tets that the unmarshalling works well.
	// TODO point a test to this
	menu := MenuRoot{}
	err := json.Unmarshal(body, &menu)
	return menu, err
}

func parseTitleJSON(body []byte) (EssensTitleRoot, error) {
	titleRoot := EssensTitleRoot{}
	err := json.Unmarshal(body, &titleRoot)
	return titleRoot, err

}

func getMensaMenuFromWeb() (MenuRoot, error) {
	// We need to set the referer header, or this won't work
	mensaMenuClient := &http.Client{}
	mensaMenuRequest, _ := http.NewRequest("GET", MENSA_URL, nil)
	mensaMenuRequest.Header.Set("Referer", "https://swp.webspeiseplan.de/Menu")

	response, err := mensaMenuClient.Do(mensaMenuRequest)

	if err != nil {
		zap.S().Warn("Can't reach mensa json. Is their service down?", err)
		return MenuRoot{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		zap.S().Warn("Can't read mensa json response body. Is their service working?", err)
		return MenuRoot{}, err
	}
	menu, err := parseJSON(body)
	if err != nil {
		zap.S().Error("Can't parse mensa json. Did the format change?", err)
		return MenuRoot{}, err
	}
	return menu, nil
}

func getMealsForToday(menu MenuRoot, day time.Time) []SpeiseplanAdvancedGericht {
	todayString := day.Format("2006-01-02")

	var todaysMenu []SpeiseplanAdvancedGericht

	for _, week := range menu.Content {
		for _, potentialMeal := range week.SpeiseplanGerichtData {
			if potentialMeal.Gericht.Aktiv == false {
				continue
			}
			potentialMealDate := potentialMeal.Gericht.Datum[:10] // is iso-formatted, this returns the day part
			if potentialMealDate == todayString {
				// We want less date precision in our db
				potentialMeal.Gericht.Datum = potentialMeal.Gericht.Datum[:10]
				todaysMenu = append(todaysMenu, potentialMeal.Gericht)
			}
		}
		if len(todaysMenu) > 0 {
			// if we found meals for today in one week we won't find them in a different week
			break
		}
	}
	return todaysMenu
}
