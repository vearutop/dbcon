package dbcon

import (
	"context"
	"database/sql"
	"strings"

	"github.com/bool64/sqluct"
)

// PostgresCompletions returns keywords, functions, tables and columns as code completions.
func PostgresCompletions(db *sql.DB) []SQLCompletion {
	//nolint:prealloc
	var (
		completions []SQLCompletion
		tables      []string
	)

	res := makeResult(context.Background(), db, "",
		"select table_name from information_schema.tables where table_schema='public';", Options{})

	if len(res.Values) == 0 {
		return completions
	}

	for _, row := range res.Values {
		table, ok := row[0].(string)
		if !ok {
			continue
		}

		completions = append(completions, SQLCompletion{
			Value: sqluct.QuoteANSI(table),
			Score: 10000,
			Meta:  "table",
			Table: sqluct.QuoteANSI(table),
		})

		completions = append(completions, SQLCompletion{
			Value: "/* table */ " + sqluct.QuoteANSI(table),
			Score: 10000,
			Meta:  "table",
		})

		tables = append(tables, table)
	}

	for _, table := range tables {
		res = makeResult(context.Background(), db, "", "SELECT column_name FROM information_schema.columns WHERE table_name = '"+table+"';", Options{})

		if len(res.Values) == 0 {
			continue
		}

		for _, row := range res.Values {
			column, ok := row[0].(string)
			if !ok {
				continue
			}

			completions = append(completions, SQLCompletion{
				Value:  sqluct.QuoteANSI(table, column),
				Score:  20000,
				Meta:   "column",
				Table:  sqluct.QuoteANSI(table),
				Column: sqluct.QuoteANSI(column),
			})

			completions = append(completions, SQLCompletion{
				Value: sqluct.QuoteANSI(column),
				Score: 20000,
				Meta:  "column",
			})
		}
	}

	completions = addCompletionsFromStringList(strings.ToUpper(`select|insert|update|delete|from|where|and|or|group|by|order|limit|offset|having|as|case|when|then|else|end|type|left|right|join|on|outer|desc|asc|union|create|table|primary|key|if|foreign|not|references|default|null|inner|cross|natural|database|drop|grant|distinct|is|in|all|alter|any|array|at|authorization|between|both|cast|check|collate|column|commit|constraint|cube|current|current_date|current_time|current_timestamp|current_user|describe|escape|except|exists|external|extract|fetch|filter|for|full|function|global|grouping|intersect|interval|into|leading|like|local|no|of|only|out|overlaps|partition|position|range|revoke|rollback|rollup|row|rows|session_user|set|some|start|tablesample|time|to|trailing|truncate|unique|unknown|user|using|values|window|with`),
		"|", "keyword", completions)

	completions = addCompletionsFromStringList(strings.ToUpper(`avg|count|first|last|max|min|sum|ucase|lcase|mid|len|round|rank|now|format|coalesce|ifnull|isnull|nvl`),
		"|", "function", completions)

	return completions
}
