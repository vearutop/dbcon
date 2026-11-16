package dbcon

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	dynhist "github.com/vearutop/dynhist-go"
)

const histogramPercent = "percent"

// Histogram describes the presentation of weighted histogram result rows.
// Rows contain minimum, maximum, count and percentage, in that order. For heatmaps rows are
// x, minimum, maximum, count and percentage of the x slice. For series (stacked) histograms rows are
// series, minimum, maximum, count and percentage of all observations.
type Histogram struct {
	Value       string                `json:"value"`
	Percent     bool                  `json:"percent,omitempty"`
	Heatmap     bool                  `json:"heatmap,omitempty"`
	Series      bool                  `json:"series,omitempty"`
	XNumeric    bool                  `json:"x_numeric,omitempty"`
	Percentiles []HistogramPercentile `json:"percentiles,omitempty"`
}

// HistogramPercentile is dynhist's bucket upper-bound estimate before squashing.
type HistogramPercentile struct {
	Percent float64 `json:"percent"`
	Value   float64 `json:"value"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
}

type histogramCollector struct {
	collector     dynhist.Collector
	resultBuckets int
	countCol      int
	valueCol      int
	info          Histogram
	percentiles   []float64
	heatmap       bool
	series        bool // Rows are split by a series label, shown as stacked histogram.
	rowsShape     bool // plot:rows
	colsShape     bool // plot:cols
	wide          bool // Value is the first column, each other column is a count of a series named by it.
	cols          []string
	xCol          int
	heatRows      []heatRow
}

// heatRow is one weighted observation in an x slice, retained to be binned into shared buckets.
type heatRow struct {
	x      string
	xNum   float64
	value  float64
	weight int
}

const histogramCount = "count"

// histogramDirectives are settings that are known only after all directives are parsed.
type histogramDirectives struct {
	found                   bool
	collectionBuckets       int
	exp                     bool
	sumWidthPow, spacingPow float64
	countName, valueName    string
	xName                   string
}

func newHistogram(statement string, cols []string) (*histogramCollector, error) {
	h := &histogramCollector{valueCol: 1, resultBuckets: dynhist.DefaultBucketsLimit}
	d := histogramDirectives{sumWidthPow: 1.2, spacingPow: 1}

	if err := h.parseDirectives(statement, &d); err != nil {
		return nil, err
	}

	if !d.found {
		return nil, nil //nolint:nilnil
	}

	if d.exp {
		h.collector.WeightFunc = dynhist.ExpWidth(d.sumWidthPow, d.spacingPow)
	}

	if d.collectionBuckets == 0 {
		d.collectionBuckets = max(200, h.resultBuckets)
	}

	if d.collectionBuckets < h.resultBuckets {
		return nil, fmt.Errorf("histogram: collect_buckets must be at least buckets")
	}

	h.collector.BucketsLimit = d.collectionBuckets

	if len(cols) < 2 {
		return nil, fmt.Errorf("histogram: expected count and value columns")
	}

	if err := h.selectColumns(cols, d); err != nil {
		return nil, err
	}

	return h, nil
}

// parseDirectives reads histogram and plot shape directives from line comments of the statement.
func (h *histogramCollector) parseDirectives(statement string, d *histogramDirectives) error {
	for i := 0; i < len(statement); {
		start := i
		i = templateSQLChunkEnd(statement, i)

		if !strings.HasPrefix(statement[start:], "--") {
			continue
		}

		lineStart := strings.LastIndexByte(statement[:start], '\n') + 1
		if strings.TrimSpace(statement[lineStart:start]) != "" {
			continue
		}

		comment := strings.TrimSpace(strings.TrimPrefix(statement[start:i], "--"))
		switch comment {
		case "plot:heatmap":
			h.heatmap = true

			continue
		case "plot:rows":
			h.rowsShape = true

			continue
		case "plot:cols":
			h.colsShape = true

			continue
		}

		if comment != "histogram" && !strings.HasPrefix(comment, "histogram:") {
			continue
		}

		d.found = true
		params := strings.TrimPrefix(strings.TrimPrefix(comment, "histogram"), ":")

		for _, param := range strings.Fields(params) {
			if err := h.applyParam(d, param); err != nil {
				return err
			}
		}
	}

	return nil
}

// applyParam applies one key or key=value parameter of a histogram directive.
func (h *histogramCollector) applyParam(d *histogramDirectives, param string) error {
	key, value, _ := strings.Cut(param, "=")

	switch key {
	case "buckets", "collect_buckets":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 10000 {
			return fmt.Errorf("histogram: %s must be between 1 and 10000", key)
		}

		if key == "buckets" {
			h.resultBuckets = n
		} else {
			d.collectionBuckets = n
		}
	case "sum_width_pow", "spacing_pow":
		p, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(p) || math.IsInf(p, 0) || p <= 0 {
			return fmt.Errorf("histogram: %s must be a number greater than 0", key)
		}

		if key == "spacing_pow" {
			d.spacingPow = p
		} else {
			d.sumWidthPow = p
		}
	case "distribution":
		d.exp = false

		switch value {
		case "linear":
			h.collector.WeightFunc = dynhist.AvgWidth
		case "latency":
			h.collector.WeightFunc = dynhist.LatencyWidth
		case "exp", "exponential":
			h.collector.WeightFunc = nil // Built after parsing, powers may follow.
			d.exp = true
		default:
			return fmt.Errorf("histogram: distribution must be linear, latency or exp")
		}
	case histogramCount, "x", "series", "value":
		if value == "" {
			return fmt.Errorf("histogram: %s requires a column name", key)
		}

		switch key {
		case histogramCount:
			d.countName = value
		case "value":
			d.valueName = value
		default:
			d.xName = value
		}
	case histogramPercent:
		if param != histogramPercent {
			return fmt.Errorf("histogram: use percent without a value")
		}

		h.info.Percent = true
	case "percentiles":
		h.percentiles = nil

		for _, item := range strings.Split(value, ",") {
			p, err := strconv.ParseFloat(item, 64)
			if err != nil || math.IsNaN(p) || math.IsInf(p, 0) || p <= 0 || p > 100 {
				return fmt.Errorf("histogram: percentiles must be comma-separated numbers greater than 0 and at most 100")
			}

			h.percentiles = append(h.percentiles, p)
		}
	default:
		return fmt.Errorf("histogram: unknown option %q", param)
	}

	return nil
}

// columnIndex finds the only column with the name.
func columnIndex(cols []string, name string) (int, error) {
	index := slices.Index(cols, name)
	if index == -1 {
		return 0, fmt.Errorf("histogram: unknown column %q", name)
	}

	if slices.Index(cols[index+1:], name) != -1 {
		return 0, fmt.Errorf("histogram: ambiguous column %q", name)
	}

	return index, nil
}

// selectColumns finds count, value and x (series) columns and the shape of the histogram.
func (h *histogramCollector) selectColumns(cols []string, d histogramDirectives) error {
	for _, selection := range []struct {
		name  string
		index *int
	}{{d.countName, &h.countCol}, {d.valueName, &h.valueCol}} {
		if selection.name == "" {
			continue
		}

		index, err := columnIndex(cols, selection.name)
		if err != nil {
			return err
		}

		*selection.index = index
	}

	if h.countCol == h.valueCol {
		return fmt.Errorf("histogram: count and value must be different columns")
	}

	h.cols = cols

	switch {
	case h.heatmap:
		h.info.Heatmap = true
	case h.colsShape && len(cols) >= 3:
		// Wide shape: value, count of series 1, count of series 2, ...
		h.wide, h.series, h.valueCol = true, true, 0
		h.info.Series = true
		h.info.Value = cols[0]

		return nil
	case h.rowsShape && (len(cols) >= 3 || d.xName != ""):
		h.series = true
		h.info.Series = true
	}

	if h.heatmap || h.series {
		h.xCol = 2

		if len(cols) < 3 && d.xName == "" {
			return fmt.Errorf("histogram: heatmap and plot:rows expect count, value and x (series) columns")
		}

		if d.xName != "" {
			index, err := columnIndex(cols, d.xName)
			if err != nil {
				return err
			}

			h.xCol = index
		}

		if h.xCol == h.countCol || h.xCol == h.valueCol {
			return fmt.Errorf("histogram: x (series) must be a different column than count and value")
		}
	}

	h.info.Value = cols[h.valueCol]

	return nil
}

func histogramNumber(value any) (float64, error) {
	if b, ok := value.([]byte); ok {
		value = string(b)
	}

	n, err := strconv.ParseFloat(fmt.Sprint(value), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("expected a finite number, got %q", fmt.Sprint(value))
	}

	return n, nil
}

func histogramWeight(value any) (int, error) {
	if b, ok := value.([]byte); ok {
		value = string(b)
	}

	maxCount := int(^uint(0) >> 1)
	if n, err := strconv.ParseInt(fmt.Sprint(value), 10, 64); err == nil && n >= 0 && n <= int64(maxCount) {
		return int(n), nil
	}

	n, err := histogramNumber(value)
	if err != nil || n < 0 || math.Trunc(n) != n || n >= float64(maxCount) {
		return 0, fmt.Errorf("expected a nonnegative integer count, got %q", fmt.Sprint(value))
	}

	return int(n), nil
}

func (h *histogramCollector) add(row []any) error {
	if h.wide {
		if row[0] == nil {
			return nil
		}

		for i := 1; i < len(row); i++ {
			if err := h.observe(row[i], row[0], h.cols[i], math.NaN()); err != nil {
				return err
			}
		}

		return nil
	}

	slice, sliceNum := "", math.NaN()

	if h.heatmap || h.series {
		slice = "NULL"
		if row[h.xCol] != nil {
			slice = fmt.Sprint(row[h.xCol])
			if b, ok := row[h.xCol].([]byte); ok {
				slice = string(b)
			}
		}

		if n, err := strconv.ParseFloat(slice, 64); err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
			sliceNum = n // Otherwise not numeric, x axis is categorical.
		}
	}

	return h.observe(row[h.countCol], row[h.valueCol], slice, sliceNum)
}

// observe adds a weighted value, keeping it for binning by slice if needed.
func (h *histogramCollector) observe(count, val any, slice string, sliceNum float64) error {
	weight, err := histogramWeight(count)
	if err != nil {
		return err
	}

	if weight == 0 || val == nil {
		return nil
	}

	value, err := histogramNumber(val)
	if err != nil {
		return err
	}

	if weight > int(^uint(0)>>1)-h.collector.Count {
		return fmt.Errorf("total histogram count overflows an integer")
	}

	h.collector.AddN(value, weight)

	if h.heatmap || h.series {
		h.heatRows = append(h.heatRows, heatRow{x: slice, xNum: sliceNum, value: value, weight: weight})
	}

	return nil
}

// heatmapValues bins retained rows into the shared bucket edges, one slice per x (or series).
// X is sorted numerically if all values are numbers, otherwise it keeps first-seen order. Series keep first-seen
// order and their percent is of all observations, so a stack sums to 100%.
func (h *histogramCollector) heatmapValues(buckets []dynhist.Bucket) [][]any {
	numeric := !h.series
	grand := 0

	for _, r := range h.heatRows {
		grand += r.weight
	}

	for _, r := range h.heatRows {
		if math.IsNaN(r.xNum) {
			numeric = false

			break
		}
	}

	h.info.XNumeric = numeric

	type slice struct {
		x      string
		xNum   float64
		counts []int
		total  int
	}

	var (
		xs  []*slice
		byX = map[string]*slice{}
	)

	for _, r := range h.heatRows {
		sl := byX[r.x]
		if sl == nil {
			sl = &slice{x: r.x, xNum: r.xNum, counts: make([]int, len(buckets))}
			byX[r.x] = sl
			xs = append(xs, sl)
		}

		// Buckets are ordered and cover every observed value.
		b := sort.Search(len(buckets), func(i int) bool { return buckets[i].Max >= r.value })
		b = min(b, len(buckets)-1)
		sl.counts[b] += r.weight
		sl.total += r.weight
	}

	if numeric {
		sort.SliceStable(xs, func(i, j int) bool { return xs[i].xNum < xs[j].xNum })
	}

	var values [][]any

	for _, sl := range xs {
		var x any = sl.x
		if numeric {
			x = sl.xNum
		}

		for i, c := range sl.counts {
			if c == 0 {
				continue
			}

			total := sl.total
			if h.series {
				total = grand
			}

			values = append(values, []any{x, buckets[i].Min, buckets[i].Max, strconv.Itoa(c), float64(c) / float64(total) * 100})
		}
	}

	return values
}

func (h *histogramCollector) collect(rows *sql.Rows, cols []string, processors map[int][]func(any) any, result *Result) {
	rowNumber := 0
	for rows.Next() {
		rowNumber++
		values, err := scanProcessedRow(rows, cols, processors)

		if err == nil {
			err = h.add(values)
		}

		if err != nil {
			result.Error = fmt.Sprintf("histogram row %d: %s", rowNumber, err)

			return
		}
	}

	if rows.Err() != nil {
		return // makeResult reports the rows error in its deferred cleanup.
	}

	collected := h.collector.Snapshot()
	if collected.Count > 0 {
		values := h.collector.Percentiles(h.percentiles...)
		for i, value := range values {
			for _, bucket := range collected.Buckets {
				if bucket.Max == value {
					h.info.Percentiles = append(h.info.Percentiles, HistogramPercentile{
						Percent: h.percentiles[i], Value: value, Min: bucket.Min, Max: bucket.Max,
					})

					break
				}
			}
		}
	}

	h.collector.Squash(h.resultBuckets)
	output := h.collector.Snapshot()

	if h.heatmap || h.series {
		first := "x"
		if h.series {
			first = "series"
		}

		result.Histogram = &h.info
		result.Columns = []string{first, "min", "max", "count", histogramPercent}
		result.Values = h.heatmapValues(output.Buckets)

		return
	}

	result.Histogram = &h.info
	result.Columns = []string{"min", "max", "count", histogramPercent}
	result.Values = make([][]any, 0, len(output.Buckets))

	for _, b := range output.Buckets {
		percent := float64(b.Count) / float64(output.Count) * 100
		result.Values = append(result.Values, []any{b.Min, b.Max, strconv.Itoa(b.Count), percent})
	}
}

func exportHistogram(result *Result, file string) error {
	path, err := resolveWorkPath(file)
	if err != nil {
		return err
	}

	f, err := os.Create(path) //nolint:gosec
	if err != nil {
		return err
	}

	writeErr := writeCSVRows(csv.NewWriter(f), result.Columns, result.Values)
	closeErr := f.Close()

	if writeErr != nil {
		return writeErr
	}

	if closeErr != nil {
		return closeErr
	}

	result.Histogram = nil
	result.Columns = []string{"csv_export"}
	result.Values = [][]any{{fmt.Sprintf("exported %d histogram buckets to %s", len(result.Values), file)}}

	return nil
}
