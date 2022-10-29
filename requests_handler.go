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

func getYAxisLabels() string {
	return `function (value) {
       switch(value) {
	case 0: 
		return '00';
	case 1:
		return '11';
	case 2:
		return '22';
	case 3:
		return '33';
	case 4:
		return '44';
	case 5:
		return '55';
	case 6:
		return '66';
	case 7:
		return '77';
	case 8:
		return '88';
	case 9:
		return '99';
	default:
	    return 'error';
       }
       }`
}

func getXAxisLabels() string {
	return `function (value) {
	let dateAsValue = new Date(1000*value);
	let hours = ('0'+dateAsValue.getHours()).slice(-2);
	let minutes = ('0'+dateAsValue.getMinutes()).slice(-2);
	return hours + ':' + minutes;
    }`
}

func updateGraphDetails() {

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

/*Takes arrays as they might be returned from a database and returns
arrays as they are needed for data visualization
*/
func prepareQueryDataForDisplay(lengths []string) []int {
	var lengthsSlice []int
	for _, element := range lengths {
		// Pattern is always "L0:", so get number by index
		lengthNumber, err := strconv.Atoi(element[1:2])
		if err != nil {
			zap.S().Errorw("Couldn't turn database entry queue length into number",
				"Value to convert", element[1],
				"error", err)
		}
		lengthsSlice = append(lengthsSlice, lengthNumber)
	}
	/*
		var timesSlice []time.Time
		for _, element := range times {
			parsedTime, err := time.Parse(time.RFC3339, element)
			zap.S().Errorw("Couldn't turn database time entry into date",
				"Value to convert", element,
				"error", err)
			timesSlice = append(timesSlice, parsedTime)
		}
		return lengthsSlice, timesSlice
	*/
	return lengthsSlice
}

/* generateGraphOfMensaTrend generates a graph out of the reports
for a specific timeframe.
Returns err if it can't generate a reportdue to lack of data
*/
func generateGraphOfMensaTrend() (string, error) {
	graphTimeFrameInSeconds := int64(30 * 60) // 30 Minutes

	queueLengthsAsStringSlice, timesSlice, err := GetAllQueueLengthReportsInTimeframe(graphTimeFrameInSeconds)

	if err == sql.ErrNoRows {
		return "", errors.New("Not enough data in timeframe")
	}
	if len(queueLengthsAsStringSlice) < 3 {
		return "", errors.New("Not enough data in timeframe")
	}

	// queueLengthsSlice, timesSlice := prepareQueryDataForDisplay(queueLengthsAsStringSlice, timestampsSlice)
	// queueLengthsSlice := prepareQueryDataForDisplay(queueLengthsAsStringSlice)
	if len(timesSlice) < 3 {
		return "", errors.New("Not enough data in timeframe")
	}

	mensaLocationObjects := GetMensaLocationSlice()
	var mensaLocationStringSlice []string
	for _, element := range *mensaLocationObjects {
		mensaLocationStringSlice = append(mensaLocationStringSlice, element.Description)
	}

	line := charts.NewLine()

	// Title
	currentTime := time.Now()
	currentTimeString := currentTime.Format("15:04")
	// xAxis.min, xAxis.max can be used to get our five minute interval

	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    fmt.Sprintf("Queue lengths for %s", currentTimeString),
			Subtitle: "Generated by @MensaQueueBot",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Type: "category",
			Data: mensaLocationStringSlice,
			// BoundaryGap: false,// TODO not supported?
			AxisLabel: &opts.AxisLabel{
				Interval:     "0", // Doesn't work for some reason
				ShowMinLabel: true,
				ShowMaxLabel: true,
				// Formatter:    opts.FuncOpts(getYAxisLabels()),
				// TODO show all ylables
			},
			SplitLine: &opts.SplitLine{
				Show: true,
			},
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Max:   currentTime.Unix(),
			Scale: true,
			Type:  "value", // Using the more natural "time" breaks (library bug)
			AxisLabel: &opts.AxisLabel{
				ShowMaxLabel: true,
				Formatter:    opts.FuncOpts(getXAxisLabels()),
			},
		}),
	)

	//yData := []int{3, 4, 2}
	var timestampsSlice []string
	for _, element := range timesSlice {
		timestampsSlice = append(timestampsSlice, strconv.FormatInt(element.Unix(), 10))
	}

	xData := timestampsSlice
	yData := queueLengthsAsStringSlice

	seriesData := make([]opts.LineData, 0)
	for i := 0; i < len(yData); i++ {
		seriesData = append(seriesData, opts.LineData{
			Value: []string{xData[i], yData[i]}})
	}

	// Put data into instance
	line.SetXAxis(xData).
		AddSeries("Category A", seriesData)
	// Where the magic happens
	f, _ := os.Create("/tmp/bar.html")
	line.Render(f)

	// After graph generation: Return the file up
	return "file:///tmp/bar.html", nil

}

func render(path string) string {
	page := rod.New().MustConnect().MustPage(path).MustWaitLoad()
	renderCommand := "() =>{return echarts.getInstanceByDom(document.getElementsByTagName('div')[1]).getDataURL()}" // this is called with javascripts .apply
	graphAsB64PNG := page.MustEval(renderCommand).Str()

	return graphAsB64PNG
}

func GenerateAndSendGraphicQueueLengthReport(chatID int) {
	if !shouldGenerateNewGraph() {
		// TODO send old graph
		return
	}
	graph_filepath, err := generateGraphOfMensaTrend()
	graphAsB64 := render(graph_filepath)
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
