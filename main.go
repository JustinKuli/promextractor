package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

var Version string

var (
	inputTSDBPath         string // INPUT_TSDB_PATH
	filterLabelName       string // FILTER_LABEL_NAME
	filterLabelExpression string // FILTER_LABEL_EXPRESSION
	outputOMPath          string // OUTPUT_OPENMETRICS_PATH
	continueOnIteratorErr bool   // CONTINUE_ON_ITERATOR_ERROR
	outputNewBlocksPath   string // OUTPUT_NEW_BLOCKS_PATH
	existingTSDBPath      string // EXISTING_TSDB_PATH
	backupTSDBPath        string // BACKUP_TSDB_PATH
	silenceShell          bool   // SILENCE_SHELLOUTS
)

func main() {
	initVars()
	log.Println("promextractor version", Version)

	createOpenMetricsFile()

	shellOut("creating blocks with promtool", "/app/promtool", "tsdb", "create-blocks-from", "openmetrics",
		outputOMPath, outputNewBlocksPath)

	if existingTSDBPath == "" {
		log.Println("EXISTING_TSDB_PATH empty, not copying blocks")
		log.Println("Complete!")
		return
	}

	shellOut("backing up existing database", "cp", "-rf", existingTSDBPath, backupTSDBPath)

	entries, err := os.ReadDir(outputNewBlocksPath)
	if err != nil {
		log.Fatalln("Error listing new blocks:", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			shellOut("moving new block "+entry.Name(), "cp", "-r",
				filepath.Join(outputNewBlocksPath, entry.Name()),
				filepath.Join(existingTSDBPath, entry.Name()),
			)
		}
	}

	log.Println("Complete!")
}

func createOpenMetricsFile() {
	log.Println("Opening database at", inputTSDBPath)

	db, err := tsdb.Open(inputTSDBPath, nil, nil, tsdb.DefaultOptions(), nil)
	if err != nil {
		log.Fatalln("Error opening database:", err)
	}
	defer handleClose(db, "database")()

	querier, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
	if err != nil {
		log.Fatalln("Error getting a querier for the database:", err)
	}
	defer handleClose(querier, "database querier")()

	matcher, err := labels.NewMatcher(labels.MatchRegexp, filterLabelName, filterLabelExpression)
	if err != nil {
		log.Fatalln("Error creating matcher to filter metrics:", err)
	}

	f, err := os.Create(outputOMPath)
	if err != nil {
		log.Fatalln("Error creating output OpenMetrics file:", err)
	}
	defer handleClose(f, "OpenMetrics file")()

	seriesCount := 0
	samplesCount := 0

	ss := querier.Select(false, nil, matcher)
	for ss.Next() {
		if seriesCount%500 == 0 {
			log.Println("Iterating over series", seriesCount)
		}

		series := ss.At()
		labels := series.Labels().Map()

		labelsList := make([]string, 0, len(labels))
		for k, v := range labels {
			if k != "__name__" {
				labelsList = append(labelsList, fmt.Sprintf("%s=%q", k, v)) // values are quoted
			}
		}

		sort.Strings(labelsList) // sort them for consistency

		metricIdentifier := fmt.Sprintf("%s{%s}", labels["__name__"], strings.Join(labelsList, ","))
		seriesCount++

		it := series.Iterator(nil)
		for it.Next() != chunkenc.ValNone {
			millis, v := it.At()

			tsSeconds := millis / 1000
			tsDecimal := millis % 1000

			fmt.Fprintf(f, "%v %v %v.%v\n", metricIdentifier, v, tsSeconds, tsDecimal)
			samplesCount++
		}

		if err := it.Err(); err != nil {
			log.Printf("Error while iterating over '%v', error: %v\n", metricIdentifier, err)
			if !continueOnIteratorErr {
				os.Exit(1)
			}
		}
	}

	if err := ss.Err(); err != nil {
		log.Printf("Error while iterating, error: %v\n", err)
		if !continueOnIteratorErr {
			os.Exit(1)
		}
	}

	ws := ss.Warnings()
	if len(ws) > 0 {
		log.Println("Warnings:", ws)
	}

	fmt.Fprintln(f, "# EOF") // Needed for the OpenMetrics format

	fmt.Printf("Wrote %v samples, from %v series.\n", samplesCount, seriesCount)
}

func handleClose(closer interface{ Close() error }, name string) func() {
	return func() {
		if err := closer.Close(); err != nil {
			log.Fatalf("Error closing %q, error: %v\n", name, err)
		}
	}
}

func shellOut(name string, cmd ...string) {
	log.Printf("Executing %q\n", strings.Join(cmd, " "))
	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()

	if !silenceShell || err != nil {
		log.Println(string(out))
	}

	if err != nil {
		log.Fatalf("Error %v: %v\n", name, err)
	}
}

func initVars() {
	if Version == "" {
		Version = "dev"
	}

	inputTSDBPath = os.Getenv("INPUT_TSDB_PATH")
	if inputTSDBPath == "" {
		inputTSDBPath = "/input/prometheus"
	}

	filterLabelName = os.Getenv("FILTER_LABEL_NAME")
	if filterLabelName == "" {
		filterLabelName = "job"
	}

	filterLabelExpression = os.Getenv("FILTER_LABEL_EXPRESSION")
	if filterLabelExpression == "" {
		filterLabelExpression = ".*foo.*"
	}

	outputOMPath = os.Getenv("OUTPUT_OPENMETRICS_PATH")
	if outputOMPath == "" {
		outputOMPath = "/tmp/open-metrics.txt"
	}

	continueOnIteratorErrStr := os.Getenv("CONTINUE_ON_ITERATOR_ERROR")
	if strings.EqualFold(continueOnIteratorErrStr, "true") {
		continueOnIteratorErr = true
	}

	outputNewBlocksPath = os.Getenv("OUTPUT_NEW_BLOCKS_PATH")
	if outputNewBlocksPath == "" {
		outputNewBlocksPath = "/tmp/new-tsdb"
	}

	existingTSDBPath = os.Getenv("EXISTING_TSDB_PATH")

	backupTSDBPath = os.Getenv("BACKUP_TSDB_PATH")
	if backupTSDBPath == "" {
		backupTSDBPath = "/output/prometheus-bak"
	}

	silenceShellStr := os.Getenv("SILENCE_SHELLOUTS")
	if strings.EqualFold(silenceShellStr, "true") {
		silenceShell = true
	}
}
