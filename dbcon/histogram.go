package dbcon

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	dynhist "github.com/vearutop/dynhist-go"
)

// Histogram describes the presentation of weighted histogram result rows.
// Rows contain minimum, maximum, count and percentage, in that order.
type Histogram struct {
	Value       string                `json:"value"`
	Percent     bool                  `json:"percent,omitempty"`
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
}

func newHistogram(statement string, cols []string) (*histogramCollector, error) {
	h := &histogramCollector{valueCol: 1, resultBuckets: dynhist.DefaultBucketsLimit}
	collectionBuckets := 0
	countName, valueName := "", ""
	found := false

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
		if comment != "histogram" && !strings.HasPrefix(comment, "histogram:") {
			continue
		}

		found = true
		params := strings.TrimPrefix(strings.TrimPrefix(comment, "histogram"), ":")
		for _, param := range strings.Fields(params) {
			key, value, _ := strings.Cut(param, "=")
			switch key {
			case "buckets", "collect_buckets":
				n, err := strconv.Atoi(value)
				if err != nil || n < 1 || n > 10000 {
					return nil, fmt.Errorf("histogram: %s must be between 1 and 10000", key)
				}

				if key == "buckets" {
					h.resultBuckets = n
				} else {
					collectionBuckets = n
				}
			case "count":
				if value == "" {
					return nil, fmt.Errorf("histogram: count requires a column name")
				}

				countName = value
			case "value":
				if value == "" {
					return nil, fmt.Errorf("histogram: value requires a column name")
				}

				valueName = value
			case "percent":
				if param != "percent" {
					return nil, fmt.Errorf("histogram: use percent without a value")
				}

				h.info.Percent = true
			case "percentiles":
				h.percentiles = nil
				for _, item := range strings.Split(value, ",") {
					p, err := strconv.ParseFloat(item, 64)
					if err != nil || math.IsNaN(p) || math.IsInf(p, 0) || p <= 0 || p > 100 {
						return nil, fmt.Errorf("histogram: percentiles must be comma-separated numbers greater than 0 and at most 100")
					}

					h.percentiles = append(h.percentiles, p)
				}
			default:
				return nil, fmt.Errorf("histogram: unknown option %q", param)
			}
		}
	}

	if !found {
		return nil, nil //nolint:nilnil
	}
	if collectionBuckets == 0 {
		collectionBuckets = max(200, h.resultBuckets)
	}
	if collectionBuckets < h.resultBuckets {
		return nil, fmt.Errorf("histogram: collect_buckets must be at least buckets")
	}

	h.collector.BucketsLimit = collectionBuckets

	if len(cols) < 2 {
		return nil, fmt.Errorf("histogram: expected count and value columns")
	}

	for _, selection := range []struct {
		name  string
		index *int
	}{{countName, &h.countCol}, {valueName, &h.valueCol}} {
		if selection.name == "" {
			continue
		}

		index := -1
		for i, name := range cols {
			if name == selection.name {
				if index != -1 {
					return nil, fmt.Errorf("histogram: ambiguous column %q", name)
				}

				index = i
			}
		}
		if index == -1 {
			return nil, fmt.Errorf("histogram: unknown column %q", selection.name)
		}

		*selection.index = index
	}
	if h.countCol == h.valueCol {
		return nil, fmt.Errorf("histogram: count and value must be different columns")
	}

	h.info.Value = cols[h.valueCol]

	return h, nil
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
	weight, err := histogramWeight(row[h.countCol])
	if err != nil {
		return err
	}
	if weight == 0 || row[h.valueCol] == nil {
		return nil
	}

	value, err := histogramNumber(row[h.valueCol])
	if err != nil {
		return err
	}
	if weight > int(^uint(0)>>1)-h.collector.Count {
		return fmt.Errorf("total histogram count overflows an integer")
	}

	// Add inserts/merges once. The default AvgWidth merge strategy depends only
	// on boundaries, so adjusting the resulting bucket is equivalent to adding
	// the same value weight times. This collector is local to one query.
	h.collector.Add(value)
	extra := weight - 1
	h.collector.Count += extra
	h.collector.Sum += float64(extra) * value

	for i := range h.collector.Buckets {
		b := &h.collector.Buckets[i]
		if value >= b.Min && value <= b.Max {
			b.Count += extra
			b.Sum += float64(extra) * value

			break
		}
	}

	return nil
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

	if h.collector.Count > 0 {
		for _, percent := range h.percentiles {
			value := h.collector.Percentile(percent)
			for _, bucket := range h.collector.Buckets {
				if bucket.Max == value {
					h.info.Percentiles = append(h.info.Percentiles, HistogramPercentile{
						Percent: percent, Value: value, Min: bucket.Min, Max: bucket.Max,
					})

					break
				}
			}
		}
	}

	h.squash()

	result.Histogram = &h.info
	result.Columns = []string{"min", "max", "count", "percent"}
	result.Values = make([][]any, 0, len(h.collector.Buckets))
	for _, b := range h.collector.Buckets {
		percent := float64(b.Count) / float64(h.collector.Count) * 100
		result.Values = append(result.Values, []any{b.Min, b.Max, strconv.Itoa(b.Count), percent})
	}
}

// squash uses the same greedy adjacent-pair merging as dynhist's AvgWidth
// collector, but only after all rows have been collected at the higher limit.
// Merge whole buckets so their counts, sums and observed ranges are preserved.
func (h *histogramCollector) squash() {
	buckets := h.collector.Buckets
	for len(buckets) > h.resultBuckets {
		merge := 1
		minWidth := dynhist.AvgWidth(buckets[0], buckets[1], h.collector.Bucket)
		for i := 2; i < len(buckets); i++ {
			width := dynhist.AvgWidth(buckets[i-1], buckets[i], h.collector.Bucket)
			if width < minWidth {
				merge = i
				minWidth = width
			}
		}

		buckets[merge-1].Max = buckets[merge].Max
		buckets[merge-1].Count += buckets[merge].Count
		buckets[merge-1].Sum += buckets[merge].Sum
		copy(buckets[merge:], buckets[merge+1:])
		buckets = buckets[:len(buckets)-1]
	}

	h.collector.Buckets = buckets
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
