package main

// End-to-end style tests that aren't automatic, but greatly speed up
// the creation of stuff that a dev can manually check afterwards

import (
	"os"
	"strconv"
	"testing"
	"time"

	"go.uber.org/zap"
)

/*
This is a manual test that sends a bunch of graphs to the tester.
These are used to verify that things look alright at different times/dates,
which are representative for users.
The Acceptance criterion is "does it look alright", which we can't automatically
test for. Thus, this.

Run this with a (copy of a) real DB
*/
func TestGenerateAWholeBunchOfGraphs(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)
	// 9:15, 10:30, 11:45, 13:00, 14:15
	//Mon, Di, Mi, Do, Fr
	// Current DB has date0
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Errorf("Couldn't generate graphs 1")
	}
	formatString := "2006-01-02T15:04:00 (MST)"
	graphTimeframeIntoPast, err := time.ParseDuration("60m")
	if err != nil {
		t.Errorf("Couldn't generate graphs 2")
	}
	graphTimeframeIntoFuture, err := time.ParseDuration("30m")
	if err != nil {
		t.Errorf("Couldn't generate graphs 3")
	}

	interestingTimes := []string{
		"2022-11-01T13:00:00 (CEST)",
		"2022-11-01T13:30:00 (CEST)",
		//"2022-11-01T14:00:00 (CEST)",
		//"2022-11-01T14:30:00 (CEST)",
		//"2022-11-01T15:00:00 (CEST)",
	}

	for _, i := range interestingTimes {
		queryTime, _ := time.ParseInLocation(formatString, i, loc)
		graphFilepath, _ := generateGraphOfMensaTrendAsHTML(queryTime.UTC(), graphTimeframeIntoPast, graphTimeframeIntoFuture)
		pathToPng, _ := renderHTMLGraphToPNG(graphFilepath)
		chatIDString, doesExist := os.LookupEnv(KEY_DEBUG_MODE)
		if !doesExist {
			zap.S().Panicf("Fatal Error: Environment variable for dev to report to not set. Set to telegram ID of dev", KEY_DEBUG_MODE)

		}
		chatID, err := strconv.Atoi(chatIDString)
		if err != nil {
			zap.S().Panicf("Fatal Error: Debug mode flag is not a telegram id", KEY_DEBUG_MODE)

		}
		stringReport := i
		SendDynamicPhoto(chatID, pathToPng, stringReport)
	}
	t.Errorf("Error to see logs")
}
