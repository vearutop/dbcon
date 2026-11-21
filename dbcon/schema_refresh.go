package dbcon

import (
	"context"

	"github.com/swaggest/usecase"
)

// staticCompletions returns the directive/value-processor completions available regardless of schema.
func staticCompletions(o Options) []SQLCompletion {
	cmp := []SQLCompletion{ //nolint:prealloc
		{Value: "-- histogram", Score: 1000, Meta: "collect weighted (count, value) rows into histogram buckets"},
		{Value: "-- histogram:buckets=20", Score: 1000, Meta: "maximum number of adaptive histogram buckets"},
		{Value: "-- histogram:buckets=all", Score: 1000, Meta: "one histogram bucket per distinct value"},
		{Value: "-- histogram:buckets=20 collect_buckets=200", Score: 1000, Meta: "collect finer buckets, then merge to the output limit"},
		{Value: "-- histogram:count=cnt value=l buckets=20", Score: 1000, Meta: "select histogram columns by name"},
		{Value: "-- histogram:percent", Score: 1000, Meta: "show percentages on histogram Y axis"},
		{Value: "-- histogram:distribution=latency", Score: 1000, Meta: "narrow buckets for low values, wider for high values"},
		{Value: "-- histogram:distribution=exp sum_width_pow=1.2 spacing_pow=1", Score: 1000, Meta: "exponentially growing buckets, tunable powers"},
		{Value: "-- histogram:percentiles=50,90,95,99", Score: 1000, Meta: "show percentile estimates from collection buckets before squashing"},
		{Value: "-- set:$tableName=my_table", Score: 1000, Meta: "set template variable for following SQL"},
		{Value: "-- for:$column in col_a, col_b", Score: 1000, Meta: "repeat statements for each expression"},
		{Value: "-- for:$column in", Score: 1000, Meta: "repeat with one -- item:expression per line"},
		{Value: "-- item:column_name", Score: 1000, Meta: "add an expression at the start of a for block"},
		{Value: "-- endfor", Score: 1000, Meta: "end repeated statements"},
		{Value: "-- plot:cols", Score: 1000, Meta: "exp cols: x, y1, y2, ..."},
		{Value: "-- plot:rows", Score: 1000, Meta: "exp cols: x, y, label"},
		{Value: "-- plot:time", Score: 1000, Meta: "x axis is UTC time (x.type=time)"},
		{Value: "-- plot:heatmap", Score: 1000, Meta: "histogram per x slice, expects count, value, x columns"},
		{Value: "-- plot:stacked_bars", Score: 1000, Meta: "stacked bars"},
		{Value: "-- plot:pie", Score: 1000, Meta: "exp cols: count, label"},
		{Value: "-- plot:value.unit=s", Score: 1000, Meta: "histogram/heatmap value is seconds, show as 1h34m2s; also x.unit, y.unit, x.format, pie.total, caption, height"},
		{Value: "-- strip", Score: 1000, Meta: "render data only"},
		{Value: "-- # ", Score: 1000, Meta: "add header"},
		{Value: "-- > ", Score: 1000, Meta: "add description"},
		{Value: "-- transpose", Score: 1000, Meta: "transpose result"},
		{Value: "-- transpose:skip_similar", Score: 1000, Meta: "transpose result for diff"},
		{Value: "-- csv:import=file.csv", Score: 1000, Meta: "import CSV as table, own statement"},
		{Value: "-- csv:import=file.csv into=table_name", Score: 1000, Meta: "import CSV into a named table"},
		{Value: "-- csv:import=file.csv header=false cols=a,b,c", Score: 1000, Meta: "import headerless CSV"},
		{Value: "-- csv:import=file.csv append=true", Score: 1000, Meta: "insert into an already existing table"},
		{Value: "-- csv:import=data_*.csv into=table_name", Score: 1000, Meta: "import multiple uniform CSVs"},
		{Value: "-- csv:export=file.csv", Score: 1000, Meta: "write result to a CSV file in working dir"},
	}

	cmp = append(cmp, o.Completions...)

	for k := range o.valueProcessor {
		cmp = append(cmp, SQLCompletion{Value: "-- " + k + ":column_name", Score: 1000, Meta: "value processor"})
	}

	return cmp
}

// buildCompletions merges each instance's schema completions with the static ones.
func buildCompletions(deps Deps, o Options) map[string][]SQLCompletion {
	static := staticCompletions(o)

	if len(deps.DBInstances()) > 1 {
		for _, v := range deps.DBInstances() {
			static = append(static, SQLCompletion{Value: "-- instance:" + v.Name, Score: 1000, Meta: "run following statements on this DB instance"})
		}
	}

	completions := map[string][]SQLCompletion{}

	for _, v := range deps.DBInstances() {
		full := make([]SQLCompletion, 0, len(v.Completions)+len(static))
		full = append(full, v.Completions...)
		full = append(full, static...)
		completions[v.Name] = full
	}

	return completions
}

// RefreshSchema re-reads table/column names from DB instances, so that tables created or
// imported after startup show up in completions and the columns directory.
func RefreshSchema(deps Deps, options ...func(*Options)) usecase.Interactor {
	o := Options{}
	for _, option := range options {
		option(&o)
	}

	type req struct {
		Instance string `query:"instance" description:"Instance to refresh, all instances if empty"`
	}

	type resp struct {
		Completions map[string][]SQLCompletion `json:"completions"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, in req, out *resp) error {
		instances := deps.DBInstances()

		for i, v := range instances {
			if in.Instance != "" && v.Name != in.Instance {
				continue
			}

			cmp, promptBase := v.loadCompletions(options...) //nolint:contextcheck // no context parameter available

			instances[i].Completions = cmp
			instances[i].PromptBase = promptBase
		}

		out.Completions = buildCompletions(deps, o)

		return nil
	})

	return u
}
