package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-rod/rod"
	"go.uber.org/zap"
)

// Used to store information about the last graph we generated/send
// to telegram. We don't need cross-reboot persistence,
// and the alternative would be storing this in the DB
var globalLatestGraphDetails graphDetails

// I'm uncertain whether this is necessary, but let's make sure nothing happens
var graphDetailMutex sync.Mutex

type graphDetails struct {
	Timestamp          time.Time
	TelegramAssignedID string
}

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

func shouldGenerateNewGraph() bool {
	maximalAcceptableTimeDeltaInSeconds := 60.0
	latestGraphTime := getTimeOfLastGraph()
	if latestGraphTime.IsZero() {
		return true
	}
	timeDelta := time.Since(latestGraphTime)
	return timeDelta.Seconds() > maximalAcceptableTimeDeltaInSeconds
}

/* createEchartOptions returns a slice of the configuration
options we want to use for our chart. For details see
https://echarts.apache.org/en/option.html
*/
func createEchartOptions() []charts.GlobalOpts {
	echartOptionsSlice := make([]charts.GlobalOpts, 0)

	// Title
	currentTime := time.Now()
	currentTimeString := currentTime.Format("15:04")

	title := charts.WithTitleOpts(opts.Title{
		Title:    fmt.Sprintf("Queue lengths for %s", currentTimeString),
		Subtitle: "Generated by @MensaQueueBot",
	})
	echartOptionsSlice = append(echartOptionsSlice, title)

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

	seriesData := make([]opts.LineData, len(yData))
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
func generateGraphOfMensaTrendAsHTML() (string, error) {
	graphTimeFrameInSeconds := int64(30 * 60) // 30 Minutes

	line := charts.NewLine()
	globalOptions := createEchartOptions()
	line.SetGlobalOptions(globalOptions...)

	xData, seriesData, err := createEchartXDataAndDataSeries(graphTimeFrameInSeconds)
	if err != nil {
		// Likely not enough data
		return "", err
	}
	line.SetXAxis(xData).
		AddSeries("Mensa Queue Lengths", seriesData)

	pathToHtmlFile := "/tmp/mensa_queue_bot_length_graph.html"

	f, _ := os.Create(pathToHtmlFile)
	line.Render(f)
	// After graph generation: Return the file up
	return "file://" + pathToHtmlFile, nil

}

/* renderHTMLGraphToPNG does exactly what it says. It expects
a path to a html file, and returns a b64 string representing
the png.
*/
func renderHTMLGraphToPNG(pathToGraphHTML string) string {
	page := rod.New().MustConnect().MustPage(pathToGraphHTML).MustWaitLoad()
	renderCommand := "() =>{return echarts.getInstanceByDom(document.getElementsByTagName('div')[1]).getDataURL()}" // this is called with javascripts .apply
	graphAsB64PNG := page.MustEval(renderCommand).Str()

	return graphAsB64PNG
}

func GenerateAndSendGraphicQueueLengthReport(chatID int) {
	if !shouldGenerateNewGraph() {
		// TODO send old graph
		// TODO make sure there's no parallelism issues here:
		// shouldGenerateNewGraph should also check whether we're currently regenerating, or something like that
		return
	}
	graph_filepath, err := generateGraphOfMensaTrendAsHTML()
	graphAsB64 := renderHTMLGraphToPNG(graph_filepath)
	zap.S().Debugw("NEW GRAPH GENERATED!", "b64", graphAsB64)
	if err != nil {
		return
	}
	// TODO handle not-enough-data case: Fall back to non-fancy graph
	// SendDynamicPhoto(chatID, photoFilePath, "TEST IMAGE")
	// TODO preload rod browser

	// Get data for
	// generate graph for now how we get data depends on that ones API

	// Upload graph to telegram

	// Send actual message to user

}
