package main

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

/*
   Sends a message to the specified user, depending on when the last reported queue length was;
   - For reported lengths within the last 5 minutes
   - For reported lengths within the last 59 minutes
   - For reported lengths on the same day
   - For no reported length on the same day
*/
func SendQueueLengthReport(chatID int, timeOfReport int, reportedQueueLength string) {
	baseMessageReportAvailable := "Current length of mensa queue is %s"
	baseMessageRelativeReportAvailable := "%d minutes ago the length was %s"
	baseMessageNoRecentReportAvailable := "No recent report, but today at %s the length was %s"
	baseMessageNoReportAvailable := "No queue length reported today."

	acceptableDeltaSinceLastReport, _ := time.ParseDuration("5m")
	timeDeltaForRelativeTimeSinceLastReport, _ := time.ParseDuration("59m")

	timestampNow := time.Now()
	timestampThen := time.Unix(int64(timeOfReport), 0)

	potsdamLocation := GetLocalLocation()
	timestampNow = timestampNow.In(potsdamLocation)
	timestampThen = timestampThen.In(potsdamLocation)

	zap.S().Infof("Loading queue length report from %s Europe/Berlin(Current time is %s Europe/Berlin)", timestampThen.Format("15:04"), timestampNow.Format("15:04"))

	var err error

	timeSinceLastReport := timestampNow.Sub(timestampThen)
	if timeSinceLastReport <= acceptableDeltaSinceLastReport {
		err = SendMessage(chatID, fmt.Sprintf(baseMessageReportAvailable, reportedQueueLength))
	} else if timeSinceLastReport <= timeDeltaForRelativeTimeSinceLastReport {
		err = SendMessage(chatID, fmt.Sprintf(baseMessageRelativeReportAvailable, int(timeSinceLastReport.Minutes()), reportedQueueLength))
	} else if timestampNow.YearDay() == timestampThen.YearDay() {
		err = SendMessage(chatID, fmt.Sprintf(baseMessageNoRecentReportAvailable, timestampThen.Format("15:04"), reportedQueueLength))
	} else {
		err = SendMessage(chatID, baseMessageNoReportAvailable)

	}
	if err != nil {
		zap.S().Error("Error while sending queue length report", err)
	}

}
