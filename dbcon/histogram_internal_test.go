package dbcon

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dynhist "github.com/vearutop/dynhist-go"
)

func TestHistogramWeightedCollection(t *testing.T) {
	for _, limit := range []int{1, 3, 20} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			h, err := newHistogram("-- histogram:buckets="+strconv.Itoa(limit)+" collect_buckets="+strconv.Itoa(limit), []string{"count", "value"})
			require.NoError(t, err)

			plain := dynhist.Collector{BucketsLimit: limit}

			for _, row := range []struct {
				weight int
				value  float64
			}{{4, 12}, {2, 36}, {7, 18}, {3, -5}, {5, 100}, {2, 13}, {3, 19}} {
				require.NoError(t, h.add([]any{row.weight, row.value}))

				for i := 0; i < row.weight; i++ {
					plain.Add(row.value)
				}
			}

			assert.Equal(t, plain.Bucket, h.collector.Bucket)
			assert.Equal(t, plain.Buckets, h.collector.Buckets)
			assert.LessOrEqual(t, len(h.collector.Buckets), limit)
		})
	}
}

func TestHistogramLargeWeightAndValidation(t *testing.T) {
	h, err := newHistogram("-- histogram", []string{"count", "value"})
	require.NoError(t, err)
	require.NoError(t, h.add([]any{int64(1_000_000_000), []byte("24")}))
	require.NoError(t, h.add([]any{0, "ignored"}))
	require.NoError(t, h.add([]any{123, nil}))
	assert.Equal(t, 1_000_000_000, h.collector.Count)
	assert.Equal(t, []dynhist.Bucket{{Min: 24, Max: 24, Count: 1_000_000_000, Sum: 24_000_000_000}}, h.collector.Buckets)

	for _, row := range [][]any{{-1, 1}, {0.5, 1}, {nil, 1}, {"NaN", 1}, {1, "NaN"}, {1, math.Inf(1)}, {1, "abc"}} {
		require.Error(t, h.add(row))
	}

	assert.Equal(t, 1_000_000_000, h.collector.Count)

	h, err = newHistogram("-- histogram", []string{"count", "value"})
	require.NoError(t, err)

	maxCount := int(^uint(0) >> 1)
	require.NoError(t, h.add([]any{strconv.Itoa(maxCount), 1}))
	require.ErrorContains(t, h.add([]any{1, 2}), "overflows")
	assert.Equal(t, maxCount, h.collector.Count)
}

func TestHistogramOptions(t *testing.T) {
	h, err := newHistogram("-- histogram:buckets=3 count=cnt value=l percent", []string{"l", "cnt"})
	require.NoError(t, err)
	assert.Equal(t, 1, h.countCol)
	assert.Equal(t, 0, h.valueCol)
	assert.Equal(t, 200, h.collector.BucketsLimit)
	assert.Equal(t, 3, h.resultBuckets)
	assert.Equal(t, Histogram{Value: "l", Percent: true}, h.info)

	for _, statement := range []string{
		"SELECT '-- histogram'",
		"SELECT '\n-- histogram\n'",
		"/*\n-- histogram\n*/ SELECT 1",
		"SELECT 1 -- histogram",
		"-- histogramming\nSELECT 1",
	} {
		h, err = newHistogram(statement, nil)
		require.NoError(t, err)
		assert.Nil(t, h)
	}

	for _, tt := range []struct {
		directive string
		cols      []string
	}{
		{"-- histogram:buckets=0", []string{"cnt", "l"}},
		{"-- histogram:buckets=-1", []string{"cnt", "l"}},
		{"-- histogram:buckets=10001", []string{"cnt", "l"}},
		{"-- histogram:buckets=x", []string{"cnt", "l"}},
		{"-- histogram:collect_buckets=0", []string{"cnt", "l"}},
		{"-- histogram:collect_buckets=-1", []string{"cnt", "l"}},
		{"-- histogram:collect_buckets=10001", []string{"cnt", "l"}},
		{"-- histogram:collect_buckets=x", []string{"cnt", "l"}},
		{"-- histogram:buckets=20 collect_buckets=10", []string{"cnt", "l"}},
		{"-- histogram:unknown=1", []string{"cnt", "l"}},
		{"-- histogram:percent=false", []string{"cnt", "l"}},
		{"-- histogram:count=", []string{"cnt", "l"}},
		{"-- histogram:value=", []string{"cnt", "l"}},
		{"-- histogram:value=missing", []string{"cnt", "l"}},
		{"-- histogram:count=cnt value=cnt", []string{"cnt", "l"}},
		{"-- histogram:value=l", []string{"l", "l"}},
		{"-- histogram", []string{"cnt"}},
	} {
		t.Run(tt.directive, func(t *testing.T) {
			_, err := newHistogram(tt.directive, tt.cols)
			require.Error(t, err)
		})
	}
}

func TestHistogramCollectionLimits(t *testing.T) {
	for _, tt := range []struct {
		directive string
		collect   int
		output    int
	}{
		{"-- histogram", 200, 20},
		{"-- histogram:buckets=3", 200, 3},
		{"-- histogram:buckets=300", 300, 300},
		{"-- histogram:buckets=2 collect_buckets=8", 8, 2},
		{"-- histogram:collect_buckets=8 buckets=2", 8, 2},
		{"-- histogram:collect_buckets=8\n-- histogram:buckets=2", 8, 2},
	} {
		t.Run(tt.directive, func(t *testing.T) {
			h, err := newHistogram(tt.directive, []string{"cnt", "value"})
			require.NoError(t, err)
			assert.Equal(t, tt.collect, h.collector.BucketsLimit)
			assert.Equal(t, tt.output, h.resultBuckets)
		})
	}
}

func TestHistogramPercentiles(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	const query = "WITH data(cnt, value) AS (VALUES (990, 10), (9, 20), (1, 100)) SELECT cnt, value FROM data"

	const directive = "-- histogram:buckets=1 collect_buckets=200 percentiles=50,90,95,99,99.9,100\n"

	// Match the library's native estimates, using weights without expanding rows.
	h, err := newHistogram(directive, []string{"cnt", "value"})
	require.NoError(t, err)

	for _, row := range [][]any{{990, 10}, {9, 20}, {1, 100}} {
		require.NoError(t, h.add(row))
	}

	result := makeResult(context.Background(), db, "test", directive+query, Options{})
	require.Empty(t, result.Error)
	require.NotNil(t, result.Histogram)
	assert.Equal(t, [][]any{{float64(10), float64(100), "1000", float64(100)}}, result.Values)
	require.Len(t, result.Histogram.Percentiles, len(h.percentiles))

	for i, percent := range h.percentiles {
		estimate := result.Histogram.Percentiles[i]
		assert.Equal(t, percent, estimate.Percent)
		assert.Equal(t, h.collector.Percentile(percent), estimate.Value)
		assert.Equal(t, estimate.Value, estimate.Max)
		assert.LessOrEqual(t, estimate.Min, estimate.Value)
	}
	// P99 must retain the finer collection estimate, not the final bucket's 100.
	assert.Equal(t, HistogramPercentile{Percent: 99, Value: 10, Min: 10, Max: 10}, result.Histogram.Percentiles[3])
	assert.Equal(t, float64(20), result.Histogram.Percentiles[4].Value)
	assert.Equal(t, float64(100), result.Histogram.Percentiles[5].Value)

	// Plot options do not affect the summary, and it is part of API/report JSON.
	plotted := makeResult(context.Background(), db, "test", "-- plot:stacked_bars\n"+directive+query, Options{})
	require.Empty(t, plotted.Error)
	assert.Equal(t, result.Histogram, plotted.Histogram)
	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded Result

	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, result.Histogram, decoded.Histogram)

	for _, query := range []string{"SELECT 1, 5 WHERE 0", "SELECT 0, 5", "SELECT 100, NULL"} {
		empty := makeResult(context.Background(), db, "test", directive+query, Options{})
		require.Empty(t, empty.Error)
		require.NotNil(t, empty.Histogram)
		assert.Empty(t, empty.Histogram.Percentiles)
		assert.Empty(t, empty.Values)
		_, err := json.Marshal(empty)
		require.NoError(t, err) // Do not expose dynhist's empty-distribution NaN in JSON.
	}
}

func TestHistogramPercentilesNearestRank(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	result := makeResult(context.Background(), db, "test",
		"-- histogram:buckets=1 percentiles=99,50,99,99.9,100\nSELECT 1, 10 UNION ALL SELECT 1, 20", Options{})
	require.Empty(t, result.Error)
	require.NotNil(t, result.Histogram)
	assert.Equal(t, []HistogramPercentile{
		{Percent: 99, Value: 20, Min: 20, Max: 20},
		{Percent: 50, Value: 10, Min: 10, Max: 10},
		{Percent: 99, Value: 20, Min: 20, Max: 20},
		{Percent: 99.9, Value: 20, Min: 20, Max: 20},
		{Percent: 100, Value: 20, Min: 20, Max: 20},
	}, result.Histogram.Percentiles)
	assert.Equal(t, [][]any{{float64(10), float64(20), "2", float64(100)}}, result.Values)
}

func TestHistogramPercentileOptions(t *testing.T) {
	h, err := newHistogram("-- histogram:percentiles=99\n-- histogram:percentiles=99.9,50", []string{"cnt", "value"})
	require.NoError(t, err)
	assert.Equal(t, []float64{99.9, 50}, h.percentiles)

	for _, value := range []string{"", "0", "-1", "101", "NaN", "Inf", "nope", "50,", ",99", "50,,99"} {
		t.Run(value, func(t *testing.T) {
			_, err := newHistogram("-- histogram:percentiles="+value, []string{"cnt", "value"})
			require.ErrorContains(t, err, "percentiles must be comma-separated")
		})
	}
}

func TestHistogramSquash(t *testing.T) {
	h, err := newHistogram("-- histogram:buckets=2 collect_buckets=8", []string{"cnt", "value"})
	require.NoError(t, err)

	for i := range 8 {
		require.NoError(t, h.add([]any{i + 1, i}))
	}

	require.Len(t, h.collector.Buckets, 8)
	total := h.collector.Bucket
	h.collector.Squash(h.resultBuckets)
	assert.Equal(t, []dynhist.Bucket{
		{Min: 0, Max: 3, Count: 10, Sum: 20},
		{Min: 4, Max: 7, Count: 26, Sum: 148},
	}, h.collector.Buckets)
	assert.Equal(t, total, h.collector.Bucket)
	// Repeated finalization must not change the buckets or totals.
	h.collector.Squash(h.resultBuckets)
	assert.Len(t, h.collector.Buckets, 2)
	assert.Equal(t, total, h.collector.Bucket)

	// Collection remains bounded even when there are more distinct values.
	h, err = newHistogram("-- histogram:buckets=2 collect_buckets=8", []string{"cnt", "value"})
	require.NoError(t, err)

	for i := range 100 {
		require.NoError(t, h.add([]any{1, i}))
		require.LessOrEqual(t, len(h.collector.Buckets), 8)
	}

	h.collector.Squash(h.resultBuckets)
	require.Len(t, h.collector.Buckets, 2)
	assert.Equal(t, 100, h.collector.Buckets[0].Count+h.collector.Buckets[1].Count)
	assert.Equal(t, float64(4950), h.collector.Buckets[0].Sum+h.collector.Buckets[1].Sum)
	assert.Equal(t, float64(0), h.collector.Buckets[0].Min)
	assert.Equal(t, float64(99), h.collector.Buckets[1].Max)
	assert.Less(t, h.collector.Buckets[0].Max, h.collector.Buckets[1].Min)
}

func TestMakeResult_Histogram(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	ctx := context.Background()
	_, err = db.ExecContext(ctx, "CREATE TABLE log (url TEXT); INSERT INTO log VALUES ('abcd'), ('abcd'), ('abcdef'), (NULL)")
	require.NoError(t, err)

	grouped := makeResult(ctx, db, "test", "-- histogram\nSELECT count(1) cnt, length(url) l FROM log GROUP BY l", Options{})
	require.Empty(t, grouped.Error)
	assert.Equal(t, &Histogram{Value: "l"}, grouped.Histogram)
	assert.Equal(t, []string{"min", "max", "count", "percent"}, grouped.Columns)
	require.Len(t, grouped.Values, 2)
	assert.Equal(t, []any{float64(4), float64(4), "2"}, grouped.Values[0][:3])
	assert.InDelta(t, 200.0/3, grouped.Values[0][3], 0.000001)
	assert.Equal(t, []any{float64(6), float64(6), "1"}, grouped.Values[1][:3])
	assert.InDelta(t, 100.0/3, grouped.Values[1][3], 0.000001)

	raw := makeResult(ctx, db, "test", "-- histogram\nSELECT 1, length(url) l FROM log", Options{})
	require.Empty(t, raw.Error)
	assert.Equal(t, grouped.Values, raw.Values)

	reordered := makeResult(ctx, db, "test", "-- histogram:count=cnt value=l percent\nSELECT length(url) l, count(1) cnt FROM log GROUP BY l", Options{})
	require.Empty(t, reordered.Error)
	assert.True(t, reordered.Histogram.Percent)
	assert.Equal(t, grouped.Values, reordered.Values)

	empty := makeResult(ctx, db, "test", "-- histogram\nSELECT 1, length(url) FROM log WHERE 0", Options{})
	require.Empty(t, empty.Error)
	require.NotNil(t, empty.Histogram)
	assert.Empty(t, empty.Values)

	for _, query := range []string{"SELECT -1, 2", "SELECT 0.5, 2", "SELECT 1, 'oops'"} {
		invalid := makeResult(ctx, db, "test", "-- histogram\n"+query, Options{})
		require.Contains(t, invalid.Error, "histogram row 1")
		assert.Empty(t, invalid.Values)
	}

	processed := makeResult(ctx, db, "test", "-- histogram\n-- double:l\nSELECT 2 cnt, 3 l", Options{
		valueProcessor: map[string]func(any) any{"double": func(v any) any {
			n, ok := v.(int64)
			require.True(t, ok)

			return n * 2
		}},
	})
	require.Empty(t, processed.Error)
	assert.Equal(t, [][]any{{float64(6), float64(6), "2", float64(100)}}, processed.Values)

	// Template loops can create a separate histogram for each selected expression.
	deps := dependencies{instances: []DBInstance{{Name: "test", Instance: db}}}
	results := queryScript(ctx, deps, "-- for:$col in length(url), length(url) * 2\n-- histogram\n-- plot:stacked_bars\nSELECT 1, $col FROM log;\n-- endfor", Options{})
	require.Len(t, results, 2)

	for _, result := range results {
		require.Empty(t, result.Error)
		require.NotNil(t, result.Histogram)
	}

	assert.Equal(t, float64(8), results[1].Values[0][0])

	squashed := makeResult(ctx, db, "test", "-- histogram:buckets=1 collect_buckets=200\nSELECT count(1), length(url) FROM log GROUP BY length(url)", Options{})
	require.Empty(t, squashed.Error)
	assert.Equal(t, [][]any{{float64(4), float64(6), "3", float64(100)}}, squashed.Values)
}

func TestHistogramCSVExport(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	f, err := os.CreateTemp(".", "histogram-test-*.csv")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	t.Cleanup(func() { _ = os.Remove(f.Name()) }) //nolint:errcheck

	result := makeResult(context.Background(), db, "test", "-- histogram\n-- csv:export="+f.Name()+"\nSELECT 10, 24", Options{})
	require.Empty(t, result.Error)
	assert.Nil(t, result.Histogram)
	assert.Equal(t, []string{"csv_export"}, result.Columns)

	data, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	assert.Equal(t, "min,max,count,percent\n24,24,10,100\n", string(data))

	result = makeResult(context.Background(), db, "test", "-- histogram:buckets=1 collect_buckets=3\n-- csv:export="+f.Name()+"\nSELECT 2, 12 UNION ALL SELECT 3, 36", Options{})
	require.Empty(t, result.Error)

	data, err = os.ReadFile(f.Name())
	require.NoError(t, err)
	assert.Equal(t, "min,max,count,percent\n12,36,5,100\n", string(data))
}

func TestHistogramHeatmap(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	// Rows are deliberately unsorted by x; x=10 has 3 observations, x=2 has 4.
	const query = "WITH d(cnt, v, x) AS (VALUES (1, 5, 10), (2, 50, 10), (3, 5, 2), (1, 50, 2)) SELECT cnt, v, x FROM d"

	result := makeResult(context.Background(), db, "test", "-- histogram:buckets=2\n-- plot:heatmap\n"+query, Options{})
	require.Empty(t, result.Error)
	require.NotNil(t, result.Histogram)
	assert.True(t, result.Histogram.Heatmap)
	assert.True(t, result.Histogram.XNumeric)
	assert.Equal(t, []string{"x", "min", "max", "count", "percent"}, result.Columns)
	assert.Equal(t, [][]any{
		{float64(2), float64(5), float64(5), "3", float64(75)},
		{float64(2), float64(50), float64(50), "1", float64(25)},
		{float64(10), float64(5), float64(5), "1", float64(1) / float64(3) * 100},
		{float64(10), float64(50), float64(50), "2", float64(2) / float64(3) * 100},
	}, result.Values)

	// x selected by name, categorical keeps first-seen order.
	result = makeResult(context.Background(), db, "test",
		"-- histogram:buckets=2 x=k\n-- plot:heatmap\nSELECT 1, 5, 'b' k, 'z' UNION ALL SELECT 1, 50, 'a', 'z' UNION ALL SELECT 1, 5, 'b', 'z'", Options{})
	require.Empty(t, result.Error)
	assert.False(t, result.Histogram.XNumeric)
	assert.Equal(t, [][]any{
		{"b", float64(5), float64(5), "2", float64(100)},
		{"a", float64(50), float64(50), "1", float64(100)},
	}, result.Values)

	// Without the heatmap directive a third column is ignored.
	plain := makeResult(context.Background(), db, "test", "-- histogram:buckets=2\n"+query, Options{})
	require.Empty(t, plain.Error)
	assert.False(t, plain.Histogram.Heatmap)
	assert.Equal(t, []string{"min", "max", "count", "percent"}, plain.Columns)

	missing := makeResult(context.Background(), db, "test", "-- histogram\n-- plot:heatmap\nSELECT 1, 2", Options{})
	assert.Contains(t, missing.Error, "expect count, value and x")
}

func TestHistogramSeries(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	// Series are in first-seen order, percent is of all observations (8 in total).
	const rowsQuery = "WITH d(cnt, v, s) AS (VALUES (2, 5, 'b'), (1, 50, 'b'), (3, 5, 'a'), (2, 50, 'a')) SELECT cnt, v, s FROM d"

	result := makeResult(context.Background(), db, "test", "-- histogram:buckets=2\n-- plot:rows\n"+rowsQuery, Options{})
	require.Empty(t, result.Error)
	require.NotNil(t, result.Histogram)
	assert.True(t, result.Histogram.Series)
	assert.False(t, result.Histogram.Heatmap)
	assert.Equal(t, []string{"series", "min", "max", "count", "percent"}, result.Columns)
	assert.Equal(t, [][]any{
		{"b", float64(5), float64(5), "2", float64(25)},
		{"b", float64(50), float64(50), "1", float64(12.5)},
		{"a", float64(5), float64(5), "3", float64(37.5)},
		{"a", float64(50), float64(50), "2", float64(25)},
	}, result.Values)

	// Wide shape: value, then a count column per series named by the column.
	wide := makeResult(context.Background(), db, "test",
		"-- histogram:buckets=2\n-- plot:cols\nWITH d(v, us, de) AS (VALUES (5, 2, 3), (50, 1, 2)) SELECT v, us, de FROM d", Options{})
	require.Empty(t, wide.Error)
	assert.True(t, wide.Histogram.Series)
	assert.Equal(t, "v", wide.Histogram.Value)
	assert.Equal(t, [][]any{
		{"us", float64(5), float64(5), "2", float64(25)},
		{"us", float64(50), float64(50), "1", float64(12.5)},
		{"de", float64(5), float64(5), "3", float64(37.5)},
		{"de", float64(50), float64(50), "2", float64(25)},
	}, wide.Values)

	// Two columns with plot:cols or plot:rows keep the plain histogram shape.
	for _, directive := range []string{"-- plot:cols\n", "-- plot:rows\n"} {
		plain := makeResult(context.Background(), db, "test", "-- histogram:buckets=2\n"+directive+"SELECT 3, 5", Options{})
		require.Empty(t, plain.Error)
		assert.False(t, plain.Histogram.Series)
		assert.Equal(t, []string{"min", "max", "count", "percent"}, plain.Columns)
	}
}

func TestHistogramAllBuckets(t *testing.T) {
	h, err := newHistogram("-- histogram:buckets=all", []string{"cnt", "value"})
	require.NoError(t, err)
	assert.Equal(t, histogramMaxBuckets, h.resultBuckets)

	// More distinct values than the default collection and result limits, each keeps its own bucket.
	for i := range 500 {
		require.NoError(t, h.add([]any{i + 1, i}))
		require.NoError(t, h.add([]any{1, i})) // Same value goes to the same bucket.
	}

	h.collector.Squash(h.resultBuckets)
	require.Len(t, h.collector.Buckets, 500)

	for i, b := range h.collector.Buckets {
		assert.Equal(t, dynhist.Bucket{Min: float64(i), Max: float64(i), Count: i + 2, Sum: float64(i * (i + 2))}, b)
	}

	_, err = newHistogram("-- histogram:buckets=all collect_buckets=5", []string{"cnt", "value"})
	require.Error(t, err)

	_, err = newHistogram("-- histogram:buckets=none", []string{"cnt", "value"})
	require.Error(t, err)
}
