package dbcon

import (
	"context"
	"database/sql"
	"strings"

	"github.com/bool64/sqluct"
	"github.com/bool64/sqluct/misc"
)

// PostgresCompletions returns keywords, functions, tables and columns as code completions.
func PostgresCompletions(db *sql.DB, options ...func(o *Options)) (_ []SQLCompletion, promptBase string) {
	//nolint:prealloc
	var (
		completions []SQLCompletion
		tables      []string
	)

	opt := &Options{}
	for _, option := range options {
		option(opt)
	}

	var tableNames map[string]bool
	if len(opt.TableNames) > 0 {
		tableNames = make(map[string]bool)
		for _, table := range opt.TableNames {
			tableNames[strings.TrimSpace(table)] = true
		}
	}

	res := makeResult(context.Background(), db, "",
		"select table_name from information_schema.tables where table_schema='public';", Options{})

	if len(res.Values) == 0 {
		return completions, ""
	}

	for _, row := range res.Values {
		table, ok := row[0].(string)
		if !ok {
			continue
		}

		if len(tableNames) > 0 && !tableNames[table] {
			continue
		}

		completions = append(completions, SQLCompletion{
			Value: sqluct.QuoteRequiredANSI(table),
			Score: 10000,
			Meta:  "table",
			Table: sqluct.QuoteRequiredANSI(table),
		})

		tables = append(tables, table)
	}

	createTables := ""

	for _, table := range tables {
		if len(tableNames) > 0 && !tableNames[table] {
			continue
		}

		res = makeResult(context.Background(), db, "", "SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name = '"+table+"';", Options{})

		if len(res.Values) == 0 {
			continue
		}

		for _, row := range res.Values {
			column, ok := row[0].(string)
			if !ok {
				continue
			}

			completions = append(completions, SQLCompletion{
				Value:  sqluct.QuoteRequiredANSI(column),
				Score:  20000,
				Meta:   sqluct.QuoteRequiredANSI(table),
				Table:  sqluct.QuoteRequiredANSI(table),
				Column: sqluct.QuoteRequiredANSI(column),
			})
		}

		createTable, err := misc.BuildPostgresCreateTable(db, "public", table)
		if err != nil {
			panic(err)
		}

		createTables += createTable + "\n"
	}

	completions = addGenericCompletions(completions)

	promptBase = "Given the following Postgres database schema, answer my next question with SQL statement, " +
		"only use columns defined in the schema:\n " + createTables + "\n\n"

	return completions, promptBase
}
