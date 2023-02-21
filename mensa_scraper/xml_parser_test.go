package mensa_scraper

import (
	"io"
	"os"
	"testing"

	"go.uber.org/zap"
)

func TestParser(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)

	file, _ := os.Open("example.xml")
	defer file.Close()
	data, _ := io.ReadAll(file)

	menu, err := parseXML(data)
	if err != nil {
		t.Errorf("Parse went wrong %e", err)
	}
	firstDate := menu.Dates[0]
	if firstDate.Index != "21.02.2023" {
		t.Errorf("First date has unexpected value")
	}
	firstOffer := firstDate.Offers[0]
	if firstOffer.Titel != "Angebot 2" {
		t.Errorf("First offer has unexpected titel")
	}
}
