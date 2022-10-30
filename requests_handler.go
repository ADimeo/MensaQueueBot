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

	"github.com/adimeo/go-echarts/v2/charts" // Custom dependency because we need features from master
	"github.com/adimeo/go-echarts/v2/opts"
	"github.com/go-rod/rod"
	"go.uber.org/zap"
)

// Used to store information about the last graph we generated/send
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

	potsdamLocation := GetLocalLocation()
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

	err := SendMessage(chatID, reportMessage)
	if err != nil {
		zap.S().Error("Error while sending queue length report", err)
	}
	return err
}

/*
getXAxisLabels returns a javascript function that converts
unix timestamps into hh:mm strings, which is the
format we want to have or labels displayed in
*/
func getXAxisLabels() string {
	return `function (value) {
	let dateAsValue = new Date(1000*value);
	let hours = ('0'+dateAsValue.getHours()).slice(-2);
	let minutes = ('0'+dateAsValue.getMinutes()).slice(-2);
	return hours + ':' + minutes;
    }`
}

func getTimeOfLastGraph() time.Time {
	return globalLatestGraphDetails.Timestamp
}

func getIdentifierOfLastGraph() string {
	return globalLatestGraphDetails.TelegramAssignedID
}

func updateGlobalLatestGraphDetails(time time.Time, newTelegramIdentifier string) {
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
func createEchartOptions(currentTime time.Time) []charts.GlobalOpts {
	echartOptionsSlice := make([]charts.GlobalOpts, 0)

	// Title
	currentTimeString := currentTime.Format("15:04")

	title := charts.WithTitleOpts(opts.Title{
		Title:    fmt.Sprintf("Queue lengths for %s", currentTimeString),
		Subtitle: "Generated by @MensaQueueBot",
	})
	echartOptionsSlice = append(echartOptionsSlice, title)

	// Grid - fix for labels being cut off
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
		// BoundaryGap: false,// TODO not supported?
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
		Max:   currentTime.Unix(),
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

/* createEchartDataSeries returns the actual data series
that echart visualizes
*/
func createEchartXDataAndDataSeries(graphTimeFrameInSeconds int64) ([]string, []opts.LineData, error) {
	// Get data
	queueLengthsAsStringSlice, timesSlice, err := GetAllQueueLengthReportsInTimeframe(graphTimeFrameInSeconds)
	if err == sql.ErrNoRows {
		return []string{}, []opts.LineData{}, errors.New("Not enough data in timeframe")
	}
	if len(queueLengthsAsStringSlice) < 3 {
		return []string{}, []opts.LineData{}, errors.New("Not enough data in timeframe")
	}
	if len(timesSlice) < 3 {
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

/* generateGraphOfMensaTrendAsHTML generates a graph out of the reports
for a specific timeframe. Writes the graph to a html file
Returns err if it can't generate a report due to lack of data
*/
func generateGraphOfMensaTrendAsHTML(graphEndTime time.Time, graphTimeFrameInSeconds int64) (string, error) {

	line := charts.NewLine()
	globalOptions := createEchartOptions(graphEndTime)
	line.SetGlobalOptions(globalOptions...)

	xData, seriesData, err := createEchartXDataAndDataSeries(graphTimeFrameInSeconds)
	if err != nil {
		// Likely not enough data
		return "", err
	}
	line.SetXAxis(xData).
		AddSeries("Mensa Queue Lengths", seriesData, charts.WithLineChartOpts(opts.LineChart{ShowSymbol: true})) // This isn't working, I think

	fileName := "mensa_queue_bot_length_graph.html"
	f, _ := os.Create("/tmp/" + fileName)

	line.Render(f)
	// Return in the format a browser would expect
	absoluteFilepath, _ := filepath.Abs(f.Name())
	return "file:///" + absoluteFilepath, nil

}

/* renderHTMLGraphToPNG does exactly what it says. It expects
a path to a html file, and writes it to a specific file.
Returns path to that file.
*/
func renderHTMLGraphToPNG(pathToGraphHTML string) (string, error) {
	page := rod.New().MustConnect().MustPage(pathToGraphHTML).MustWaitLoad()
	renderCommand := "() =>{return echarts.getInstanceByDom(document.getElementsByTagName('div')[1]).getDataURL()}" // this is called with javascripts .apply
	// Data is in the format data:image/png;base64,iVBORw0KGgoAAAANSUhEU...
	commandJson := page.MustEval(renderCommand)

	graphAsB64PNG := commandJson.Str()
	// So cut away the first 22 symbols, and b64decode the rest
	decodedPngData, err := base64.StdEncoding.DecodeString(graphAsB64PNG[22:])
	if err != nil {
		return "", err
	}
	pathToPng := "/tmp/mensa_queue_bot_length_graph.png"
	os.WriteFile(pathToPng, []byte(decodedPngData), 0666) //Read and write permissions

	return pathToPng, nil
}

func sendExistingGraphicQueueLengthReport(chatID int,
	timeOfLatestReport int, reportedQueueLength string, oldGraphIdentifier string) error {
	stringReport := generateSimpleLengthReportString(timeOfLatestReport, reportedQueueLength)
	err := SendStaticWebPhoto(chatID, oldGraphIdentifier, stringReport)
	return err
}

func sendNewGraphicQueueLengthReport(chatID int,
	timeOfLatestReport int, reportedQueueLength string) error {

	graphTimeFrameInSeconds := int64(30 * 60) // 30 Minutes
	graphEndTime := time.Now()
	graphFilepath, err := generateGraphOfMensaTrendAsHTML(graphEndTime, graphTimeFrameInSeconds)
	if err != nil {
		// Likely lack of data
		// Fallback to simple report
		return sendQueueLengthReport(chatID, timeOfLatestReport, reportedQueueLength)
	}
	pathToPng, err := renderHTMLGraphToPNG(graphFilepath)
	if err != nil {
		zap.S().Error("Couldn't render /jetze html to png", err)
		// Might be a parallelism issue?
		// Fallback to simple report
		return sendQueueLengthReport(chatID, timeOfLatestReport, reportedQueueLength)
	}
	stringReport := generateSimpleLengthReportString(timeOfLatestReport, reportedQueueLength)
	newTelegramIdentifier, err := SendDynamicPhoto(chatID, pathToPng, stringReport)
	updateGlobalLatestGraphDetails(graphEndTime, newTelegramIdentifier)
	return err
}

func GenerateAndSendGraphicQueueLengthReport(chatID int) {
	timeOfLatestReport, reportedQueueLength := GetLatestQueueLengthReport()
	if !shouldGenerateNewGraph(int64(timeOfLatestReport)) {
		// Parallelism issue with multiple graphs being generated at the same
		// time considered unlikely enough not to handle.
		oldGraphIdentifier := getIdentifierOfLastGraph()
		sendExistingGraphicQueueLengthReport(chatID, timeOfLatestReport, reportedQueueLength, oldGraphIdentifier)
		// TODO handle
	} else {
		sendNewGraphicQueueLengthReport(chatID,
			timeOfLatestReport, reportedQueueLength)
		// TODO handle
	}

	// TODO preload rod browser

	// TODO log all the things

}
