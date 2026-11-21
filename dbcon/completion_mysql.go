package dbcon

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/bool64/sqluct"
)

// MySQLCompletions returns keywords, functions, tables and columns as code completions.
func MySQLCompletions(db *sql.DB, options ...func(o *Options)) (_ []SQLCompletion, promptBase string) {
	var completions []SQLCompletion //nolint:prealloc

	opt := &Options{}
	for _, option := range options {
		option(opt)
	}

	tableNames := make(map[string]bool, len(opt.TableNames))
	for _, table := range opt.TableNames {
		tableNames[strings.TrimSpace(table)] = true
	}

	res := makeResult(context.Background(), db, "",
		"SELECT table_name, column_name FROM information_schema.columns WHERE table_schema = DATABASE() "+
			"ORDER BY table_name, ordinal_position", Options{})

	var (
		schema   strings.Builder
		lastName string
	)

	for _, row := range res.Values {
		if len(row) < 2 {
			continue
		}

		table, column := sqlString(row[0]), sqlString(row[1])
		if table == "" || column == "" || (len(tableNames) > 0 && !tableNames[table]) {
			continue
		}

		qTable := sqluct.QuoteRequiredBackticks(table)

		if table != lastName {
			if lastName != "" {
				schema.WriteString(");\n")
			}

			lastName = table

			completions = append(completions, SQLCompletion{Value: qTable, Score: 10000, Meta: "table", Table: qTable})

			schema.WriteString("CREATE TABLE " + qTable + " (")
		} else {
			schema.WriteString(", ")
		}

		qColumn := sqluct.QuoteRequiredBackticks(column)
		schema.WriteString(qColumn)

		completions = append(completions, SQLCompletion{
			Value: qColumn, Score: 20000, Meta: qTable, Table: qTable, Column: qColumn,
		})
	}

	if lastName != "" {
		schema.WriteString(");\n")
	}

	completions = addGenericCompletions(completions)

	promptBase = "Given the following MySQL database schema, answer my next question with SQL statement, " +
		"only use columns defined in the schema:\n " + schema.String() + "\n\n"

	return completions, promptBase
}

// sqlString converts a text value as returned by DB drivers, string or []byte.
func sqlString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case nil:
		return ""
	default:
		return fmt.Sprint(s)
	}
}
