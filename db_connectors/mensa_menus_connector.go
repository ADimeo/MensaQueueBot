package db_connectors

import (
	"database/sql"
	"time"

	"go.uber.org/zap"
)

type DBOfferInformation struct {
	Title       string
	Description string
	Time        time.Time
	Counter     int
}

// time, title, decsription, counter

func GetLatestMensaOffers() ([]DBOfferInformation, error) {
	queryString := `SELECT time, title, description, counter FROM mensaMenus WHERE 
	counter == (SELECT MAX(counter) FROM mensaMenus);`
	db := GetDBHandle()

	var latestOffers []DBOfferInformation

	rows, err := db.Query(queryString)
	if err != nil {
		zap.S().Errorf("Error while querying for latest mensa offers", err)
		return latestOffers, err
	}
	defer rows.Close()

	for rows.Next() {
		var latestOffer DBOfferInformation
		if err = rows.Scan(&latestOffer.Time, &latestOffer.Title, &latestOffer.Description, &latestOffer.Counter); err != nil {
			zap.S().Errorf("Error scanning for latest mensa menus, likely data type mismatch", err)
		}
		latestOffers = append(latestOffers, latestOffer)
	}

	return latestOffers, nil
}

func InsertMensaMenu(offerToInsert *DBOfferInformation) error {
	queryString := "INSERT INTO mensaMenus(time, title, description, counter) VALUES(?,?,?,?);"
	db := GetDBHandle()

	DBMutex.Lock()
	_, err := db.Exec(queryString, offerToInsert.Time, offerToInsert.Title, offerToInsert.Description, offerToInsert.Counter)
	DBMutex.Unlock()
	return err
}

func GetMensaMenuCounter() (int, error) {
	queryString := "SELECT COALESCE(MAX(counter), -1) FROM mensaMenus;"
	db := GetDBHandle()

	var counterValue int

	if err := db.QueryRow(queryString).Scan(&counterValue); err != nil {
		if err == sql.ErrNoRows {
			zap.S().Error("No rows returned when querying for latest queue length report")
			return 0, nil
		} else {
			zap.S().Error("Error while querying for latest report", err)
			return -1, err

		}
	}
	return counterValue, nil
}
