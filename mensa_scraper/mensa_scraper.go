package mensa_scraper

import (
	"encoding/xml"
	"io"
	"net/http"

	"go.uber.org/zap"
)

const MENSA_URL string = "https://xml.stw-potsdam.de/xmldata/gs/xml.php"

// Structs for representing the menu in xml
type OfferInformation struct {
	XMLName     xml.Name `xml:"angebotnr"`
	Index       string   `xml:"index,attr"`
	Titel       string   `xml:"titel"`
	Description string   `xml:"beschreibung"`
}

type DateInformation struct {
	XMLName xml.Name           `xml:"datum"`
	Index   string             `xml:"index,attr"`
	Offers  []OfferInformation `xml:"angebotnr"`
}

type MenuRoot struct {
	XMLName xml.Name          `xml:"menu"`
	Dates   []DateInformation `xml:"datum"`
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
		zap.S().Warnf("Can't reach mensa XML. Is their service down?", err)
		return MenuRoot{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		zap.S().Warnf("Can't read mensa xml response body. Is their service working?", err)
		return MenuRoot{}, err
	}
	menu, err := parseXML(body)
	if err != nil {
		zap.S().Errorf("Can't parse mensa xml. Did the format change?", err)
		return MenuRoot{}, err
	}
	return menu, nil
}
