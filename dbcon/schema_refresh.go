package dbcon

import (
	"context"

	"github.com/bool64/sqluct"
	"github.com/swaggest/usecase"
)

// staticCompletions returns the directive/value-processor completions available regardless of schema.
func staticCompletions(o Options) []SQLCompletion {
	cmp := []SQLCompletion{ //nolint:prealloc
		{Value: "-- plot:cols", Score: 1000, Meta: "exp cols: x, y1, y2, ..."},
		{Value: "-- plot:rows", Score: 1000, Meta: "exp cols: x, y, label"},
		{Value: "-- plot:time", Score: 1000, Meta: "time series"},
		{Value: "-- plot:time_axis_fmt={YYYY}-{MM}", Score: 1000, Meta: "time axis format, default {HH}:{mm}\\n{YYYY}-{MM}-{DD}"},
		{Value: "-- plot:caption=My dataset", Score: 1000, Meta: "plot caption"},
		{Value: "-- plot:height=360px", Score: 1000, Meta: "plot height"},
		{Value: "-- plot:stacked_bars", Score: 1000, Meta: "stacked bars"},
		{Value: "-- plot:pie", Score: 1000, Meta: "exp cols: count, label"},
		{Value: "-- plot:pie:total=X", Score: 1000, Meta: "override total count"},
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
		Instance instance `query:"instance" description:"Instance to refresh, all instances if empty"`
	}

	type resp struct {
		Completions map[string][]SQLCompletion `json:"completions"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, in req, out *resp) error {
		instances := deps.DBInstances()

		for i, v := range instances {
			if in.Instance != "" && v.Name != string(in.Instance) {
				continue
			}

			var (
				cmp        []SQLCompletion
				promptBase string
			)

			switch v.Dialect { //nolint:exhaustive
			case sqluct.DialectSQLite3:
				cmp, promptBase = SqliteCompletions(v.Instance, options...) //nolint:contextcheck // no context parameter available
			case sqluct.DialectPostgres:
				cmp, promptBase = PostgresCompletions(v.Instance, options...) //nolint:contextcheck // no context parameter available
			}

			instances[i].Completions = cmp
			instances[i].PromptBase = promptBase
		}

		out.Completions = buildCompletions(deps, o)

		return nil
	})

	return u
}
