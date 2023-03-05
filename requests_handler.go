package main

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ADimeo/MensaQueueBot/db_connectors"
	"github.com/ADimeo/MensaQueueBot/telegram_connector"
	"github.com/ADimeo/MensaQueueBot/utils"
	"github.com/adimeo/go-echarts/v2/charts" // Custom dependency because we need features from their master that aren't published yet
	"github.com/adimeo/go-echarts/v2/opts"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"go.uber.org/zap"
)

// Used to store information about the last graph we generated/sent
// to telegram. We don't need cross-reboot persistence,
// and the alternative would be storing this in the DB
var globalLatestGraphDetails graphDetails

type graphDetails struct {
	Timestamp          time.Time
	TelegramAssignedID string
}

/* generateSimpleLengthReportString generates the text of a report that is sent
to a user, depending on when the last reported queue length was:

   - For reported lengths within the last 5 minutes
   - For reported lengths within the last 59 minutes
   - For reported lengths on the same day
   - For no reported length on the same day
*/
func generateSimpleLengthReportString(timeOfReport int, reportedQueueLength string) string {
	baseMessageReportAvailable := "Current length of mensa queue is %s"
	baseMessageRelativeReportAvailable := "%d minutes ago the length was %s"
	baseMessageNoRecentReportAvailable := "No recent report, but today at %s the length was %s"
	baseMessageNoReportAvailable := "No queue length reported today."

	acceptableDeltaSinceLastReport, _ := time.ParseDuration("5m")
	timeDeltaForRelativeTimeSinceLastReport, _ := time.ParseDuration("59m")

	timestampNow := time.Now()
	timestampThen := time.Unix(int64(timeOfReport), 0)

	potsdamLocation := utils.GetLocalLocation()
	timestampNow = timestampNow.In(potsdamLocation)
	timestampThen = timestampThen.In(potsdamLocation)

	zap.S().Infof("Generating queueLengthReport with report from %s Europe/Berlin(Current time is %s Europe/Berlin)", timestampThen.Format("15:04"), timestampNow.Format("15:04"))

	timeSinceLastReport := timestampNow.Sub(timestampThen)
	if timeSinceLastReport <= acceptableDeltaSinceLastReport {
		return fmt.Sprintf(baseMessageReportAvailable, reportedQueueLength)
	} else if timeSinceLastReport <= timeDeltaForRelativeTimeSinceLastReport {
		return fmt.Sprintf(baseMessageRelativeReportAvailable, int(timeSinceLastReport.Minutes()), reportedQueueLength)
	} else if timestampNow.YearDay() == timestampThen.YearDay() {
		return fmt.Sprintf(baseMessageNoRecentReportAvailable, timestampThen.Format("15:04"), reportedQueueLength)
	} else {
		return baseMessageNoReportAvailable
	}
}

/*
SendQueueLengthReport sends a message to the specified user, depending on when the last reported queue length was.
See generateSimpleLengthReportString for message creation logic.
*/
func sendQueueLengthReport(chatID int, timeOfReport int, reportedQueueLength string) error {
	reportMessage := generateSimpleLengthReportString(timeOfReport, reportedQueueLength)

	err := telegram_connector.SendMessage(chatID, reportMessage)
	if err != nil {
		zap.S().Error("Error while sending queue length report", err)
	}
	return err
}

/*
getXAxisLabels returns a javascript function that converts
unix timestamps into hh:mm strings, which is the
format we want to have our labels displayed in
*/
func getXAxisLabels() string {
	return `function (value) {
	let dateAsValue = new Date(1000*value);
	let hhmmString = dateAsValue.toLocaleTimeString('de-DE', {timeZone: 'Europe/Berlin', timeStyle: 'short'});
	return hhmmString;
    }`
}

/*
getTimeOfLastGraph returns when the currently active graph
was generated
*/
func getTimeOfLastGraph() time.Time {
	return globalLatestGraphDetails.Timestamp
}

/*
getIdentifierOfLastGraph returns a string that
we can send to telegram which it will interpret
as this graph image
*/
func getIdentifierOfLastGraph() string {
	return globalLatestGraphDetails.TelegramAssignedID
}

/*
updateGlobalLatestGraphDetails is used to keep the active
graph details up to date
*/
func updateGlobalLatestGraphDetails(time time.Time, newTelegramIdentifier string) {
	zap.S().Debugf("Updated currently active graph from %s to %s",
		globalLatestGraphDetails.Timestamp.Format("15:04:05"),
		time.Format("15:04:05"))

	globalLatestGraphDetails.Timestamp = time
	globalLatestGraphDetails.TelegramAssignedID = newTelegramIdentifier
}

/* shouldGenerateNewGraph returns true if we should
generate a new graph. We should generate a new graph
if any of these is true:
- No graph currently exists
- The latest queue length report is newer than the
latest grah
- The latest graph is older than one minute

*/
func shouldGenerateNewGraph(timeOfReport int64) bool {
	latestGraphTime := getTimeOfLastGraph()
	if latestGraphTime.IsZero() {
		return true
	}
	// Check if the latest report is newer than the latest graph
	// There'll always be _some_ risk or race-condition style inconsistencies,
	// if a report happens exactly after querying here, and just before
	// generating the graph, but we accept that edge case
	latestReportTime := time.Unix(timeOfReport, 0)
	if latestGraphTime.Before(latestReportTime) {
		return true
	}

	// Graph is older than one minute
	maximalAcceptableTimeDeltaInSeconds := 60.0
	timeDelta := time.Since(latestGraphTime)
	return timeDelta.Seconds() > maximalAcceptableTimeDeltaInSeconds
}

/* createEchartOptions returns a slice of the configuration
options we want to use for our chart. For details see
https://echarts.apache.org/en/option.html
*/
func createEchartOptions(currentTime time.Time, graphEndTime time.Time, graphStartTime time.Time) []charts.GlobalOpts {
	echartOptionsSlice := make([]charts.GlobalOpts, 0)

	// Title
	mensaLocation, _ := time.LoadLocation("Europe/Berlin")
	timeInMensaTimezone := currentTime.In(mensaLocation).Format("15:04")

	title := charts.WithTitleOpts(opts.Title{
		Title:    fmt.Sprintf("Queue lengths for %s", timeInMensaTimezone),
		Subtitle: "Generated by @MensaQueueBot",
	})
	echartOptionsSlice = append(echartOptionsSlice, title)

	// Legend
	legend := charts.WithLegendOpts(opts.Legend{
		Show: true,
		Left: "right",
	})
	echartOptionsSlice = append(echartOptionsSlice, legend)

	// Grid - fix for labels being cut off
	// This is what requires our custom dependency:
	// It's not supported in echarts v2.2.4
	grid := charts.WithGridOpts(opts.Grid{
		ContainLabel: true})

	echartOptionsSlice = append(echartOptionsSlice, grid)

	// yAxis Options
	mensaLocationObjects := GetMensaLocationSlice()
	var yAxiLabelStringSlice []string
	for _, singleMensaLocation := range *mensaLocationObjects {
		yAxiLabelStringSlice = append(yAxiLabelStringSlice, singleMensaLocation.Description)
	}

	yAxis := charts.WithYAxisOpts(opts.YAxis{
		Type: "category",
		Data: yAxiLabelStringSlice,
		// BoundaryGap: false,// I'd like this option, but it's currently not supported in echarts master
		AxisLabel: &opts.AxisLabel{
			Interval:     "0",
			ShowMinLabel: true,
			ShowMaxLabel: true,
			Show:         true,
		},
		SplitLine: &opts.SplitLine{
			Show: true,
		},
	})

	echartOptionsSlice = append(echartOptionsSlice, yAxis)

	//xAxisOptions
	xAxis := charts.WithXAxisOpts(opts.XAxis{
		Max:   graphEndTime.Unix(),
		Min:   graphStartTime.Unix(),
		Scale: true,
		Type:  "value", // Using the more natural "time" breaks stuff (library bug, https://github.com/go-echarts/go-echarts/issues/194)
		AxisLabel: &opts.AxisLabel{
			ShowMaxLabel: true,
			Formatter:    opts.FuncOpts(getXAxisLabels()),
			Show:         true,
		},
	})

	echartOptionsSlice = append(echartOptionsSlice, xAxis)
	return echartOptionsSlice
}

/* convertTimesSliceToTimestamps takes a slice of time.Time and
returns a slice of string representations of the unix timestamps
of those times, which is the format that the egraph expects
*/
func convertTimesSliceToTimestampsSlice(timesSlice []time.Time) []string {
	var timestampsSlice []string
	for _, element := range timesSlice {
		timestampsSlice = append(timestampsSlice, strconv.FormatInt(element.Unix(), 10))
	}
	return timestampsSlice
}

/*normalizeTimesToTodaysTimestamps takes a list of times and
returns a list of strings with unix timestamps, one per time.
These unix timestamps
- represent the time of the original time
- But their date is set to today

Importantly, these unix timestamps are timezone aware for the
mensa timezone, so if a timestamp showed hh:mm when created,
it will show the same hh:mm but for the current date

This is useful to display at what times events that stretch
over multiple days happened.
*/
func normalizeTimesToTodaysTimestamps(today time.Time, timesSlice []time.Time) []string {
	// Daylight saving time adds some steps to this...
	var timestampsSlice []string
	for _, element := range timesSlice {
		timeInMensaTimezone := element.In(utils.GetLocalLocation())
		normalizedTime := time.Date(today.Year(), today.Month(), today.Day(),
			timeInMensaTimezone.Hour(), timeInMensaTimezone.Minute(), timeInMensaTimezone.Second(), 0,
			utils.GetLocalLocation())
		timestampsSlice = append(timestampsSlice, strconv.FormatInt(normalizedTime.Unix(), 10))
	}
	return timestampsSlice
}

/*
createEchartDataSeries returns the actual data series for todays report
that echart visualizes
*/
func createEchartXDataAndDataSeries(nowUTC time.Time, dataTimeframe time.Duration) ([]string, []opts.LineData, error) {
	// Get data
	queueLengthsAsStringSlice, timesSlice, err := db_connectors.GetAllQueueLengthReportsInTimeframe(nowUTC, dataTimeframe)
	if err == sql.ErrNoRows {
		return []string{}, []opts.LineData{}, errors.New("Not enough data in timeframe")
	}

	// Create xData
	xData := convertTimesSliceToTimestampsSlice(timesSlice)

	// creat data series
	yData := queueLengthsAsStringSlice

	seriesData := make([]opts.LineData, 0)
	for i := 0; i < len(yData); i++ {
		seriesData = append(seriesData, opts.LineData{
			Value: []string{xData[i], yData[i]}})
	}
	return xData, seriesData, nil
}

/*
getHistoricalSeriesForToday returns an array of datapoints that can be used to create a scatterchart.
Each datapoint contains a timestamp for today, and a queue length report string. The array contains data that

- Was generated within the last 30 days (Variable within function decides this length)
- created in the time between now - timeIntoPast and now.timeIntoFuture, but on all days within the inverval
*/
func getHistoricalSeriesForToday(todayUTC time.Time, timeIntoPast time.Duration, timeIntoFuture time.Duration) ([]opts.ScatterData, error) {
	historicalGraphTimeFrameInDays := int8(30)

	queueLengthsAsStringSlice, timesSlice, err := db_connectors.GetQueueLengthReportsByWeekdayAndTimeframe(historicalGraphTimeFrameInDays, todayUTC, timeIntoPast, timeIntoFuture)
	if err == sql.ErrNoRows {
		return []opts.ScatterData{}, errors.New("No historical data found")
	}
	// Normalize timestamps for today

	// creat data series
	xData := normalizeTimesToTodaysTimestamps(todayUTC, timesSlice)
	yData := queueLengthsAsStringSlice

	seriesData := make([]opts.ScatterData, 0)
	for i := 0; i < len(yData); i++ {
		seriesData = append(seriesData, opts.ScatterData{
			Value: []string{xData[i], yData[i]}})
	}
	return seriesData, nil

}

/* buildHistoricalScatterChart returns an echartsScatterchars of historical reports within the
given intervals (with the now moment being in the middle of the two vlaues
*/
func buildHistoricalScatterChart(todayUTC time.Time, timeIntoPast time.Duration, timeIntoFuture time.Duration) (*charts.Scatter, error) {
	scatter := charts.NewScatter()

	historicalSeries, err := getHistoricalSeriesForToday(todayUTC, timeIntoPast, timeIntoFuture)
	if err != nil {
		return scatter, err
	}
	scatter.AddSeries("Reports from last 30 days", historicalSeries)
	return scatter, nil
}

/* generateGraphOfMensaTrendAsHTML generates a graph out of the reports
for a specific timeframe. Writes the graph to a html file
Returns err if it can't generate a report due to lack of data
*/
func generateGraphOfMensaTrendAsHTML(graphCenterTimeUTC time.Time, timeIntoPast time.Duration, timeIntoFuture time.Duration) (string, error) {
	line := charts.NewLine()
	globalOptions := createEchartOptions(graphCenterTimeUTC,
		graphCenterTimeUTC.Add(-timeIntoPast),
		graphCenterTimeUTC.Add(timeIntoFuture))
	line.SetGlobalOptions(globalOptions...)

	xData, seriesData, err := createEchartXDataAndDataSeries(graphCenterTimeUTC, timeIntoPast)
	if err != nil {
		// Likely not enough data
		zap.S().Debug("Not enough data to create /jetze graph", err)
	}
	line.SetXAxis(xData).
		AddSeries("Reports from today", seriesData).
		SetSeriesOptions(
			charts.WithMarkLineNameXAxisItemOpts(opts.MarkLineNameXAxisItem{
				Name:  "Now",
				XAxis: graphCenterTimeUTC.Unix(),
			}),
			charts.WithMarkLineStyleOpts(opts.MarkLineStyle{
				Symbol: []string{"none"},
				Label: &opts.Label{
					Formatter: opts.FuncOpts("function (value){return 'Now'}"),
					Show:      true,
				}}),
		)

	fileName := "mensa_queue_bot_length_graph.html"
	f, err := os.Create("/tmp/" + fileName)
	if err != nil {
		zap.S().Error("Couldn't create /jetze .html file, even though we have enough data", err)
		return "", err
	}
	// Add historical data
	historicalScatterChart, err := buildHistoricalScatterChart(graphCenterTimeUTC, timeIntoPast, timeIntoFuture)
	if err != nil {
		zap.S().Error("Couldn't scatter chart with historical data, displaying only todays reports", err)
	} else {
		line.Overlap(historicalScatterChart)
	}

	line.Render(f)
	// Return in the format a browser would expect
	absoluteFilepath, _ := filepath.Abs(f.Name())
	return "file:///" + absoluteFilepath, nil
}

/* renderHTMLGraphToPNG does exactly what it says. It expects
a path to a html file, and writes it to a specific file.
Returns path to that file.

Rendering happens via an external browser. Extraction to PNG via
echarts getDataURL method
*/
func renderHTMLGraphToPNG(pathToGraphHTML string) (string, error) {
	u := launcher.New().Bin("/usr/bin/google-chrome").MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	page := browser.MustPage(pathToGraphHTML).MustWaitLoad()
	renderCommand := "() =>{return echarts.getInstanceByDom(document.getElementsByTagName('div')[1]).getDataURL()}" // this is called with javascripts .apply
	commandJsonResponse := page.MustEval(renderCommand)
	browser.MustClose()

	graphAsB64PNG := commandJsonResponse.Str()
	// Data is in the format data:image/png;base64,iVBORw0KGgoAAAANSUhEU...
	// So cut away the first 22 symbols, and b64decode the rest
	decodedPngData, err := base64.StdEncoding.DecodeString(graphAsB64PNG[22:])
	if err != nil {
		zap.S().Error("Render html->png failed", err)
		return "", err
	}
	pathToPng := "/tmp/mensa_queue_bot_length_graph.png"
	os.WriteFile(pathToPng, []byte(decodedPngData), 0666) //Read and write permissions

	return pathToPng, nil
}

/*
sendExistingGraphicQueueLengthReport sends the currently active graph
to our users. That way we don't have to regenerate our graphs on every request
*/
func sendExistingGraphicQueueLengthReport(chatID int,
	timeOfLatestReport int, reportedQueueLength string, oldGraphIdentifier string) error {
	stringReport := generateSimpleLengthReportString(timeOfLatestReport, reportedQueueLength)
	err := telegram_connector.SendStaticWebPhoto(chatID, oldGraphIdentifier, stringReport)
	return err
}

/*
sendNewGraphicQueueLengthReport generates a new graph and sends
it to the user. Gracefully falls back to string reports if we
lack data or if errors occur
*/
func sendNewGraphicQueueLengthReport(chatID int,
	timeOfLatestReport int, reportedQueueLength string) error {

	graphNowLineUTC := time.Now().UTC()
	graphTimeframeIntoPast, _ := time.ParseDuration("60m")
	graphTimeframeIntoFuture, _ := time.ParseDuration("30m")

	graphFilepath, err := generateGraphOfMensaTrendAsHTML(graphNowLineUTC, graphTimeframeIntoPast, graphTimeframeIntoFuture)
	if err != nil {
		// Likely lack of data
		// Fallback to simple report
		zap.S().Debug("Falling back to sending non-graphic report")
		return sendQueueLengthReport(chatID, timeOfLatestReport, reportedQueueLength)
	}

	pathToPng, err := renderHTMLGraphToPNG(graphFilepath)
	if err != nil {
		zap.S().Error("Couldn't render /jetze html to png, fallback to text report", err)
		// Might be a parallelism issue?
		// Fallback to simple report
		return sendQueueLengthReport(chatID, timeOfLatestReport, reportedQueueLength)
	}
	stringReport := generateSimpleLengthReportString(timeOfLatestReport, reportedQueueLength)
	newTelegramIdentifier, err := telegram_connector.SendDynamicPhoto(chatID, pathToPng, stringReport)
	updateGlobalLatestGraphDetails(graphNowLineUTC, newTelegramIdentifier)
	return err
}

/*
The handling of a /jetze request. If possible we will try to send a graphic
report, but fallbacks to text reports exist. Caching exists,
with the time window of the cache being defined in shouldGenerateNewGraph

*/
func GenerateAndSendGraphicQueueLengthReport(chatID int) {
	timeOfLatestReport, reportedQueueLength := db_connectors.GetLatestQueueLengthReport()
	if !shouldGenerateNewGraph(int64(timeOfLatestReport)) {
		// Parallelism issue with multiple graphs being generated at the same
		// time considered unlikely enough not to handle.
		zap.S().Debug("Sending existing graph for graphic report")
		oldGraphIdentifier := getIdentifierOfLastGraph()
		err := sendExistingGraphicQueueLengthReport(chatID, timeOfLatestReport, reportedQueueLength, oldGraphIdentifier)
		if err != nil {
			zap.S().Error("Something failed while sending an existing report", err)
		}
	} else {
		telegram_connector.SendTypingIndicator(chatID)
		zap.S().Debug("Creating new graph for graphic report")
		err := sendNewGraphicQueueLengthReport(chatID,
			timeOfLatestReport, reportedQueueLength)
		if err != nil {
			zap.S().Error("Something failed while sending a new report", err)
		}
	}
}
