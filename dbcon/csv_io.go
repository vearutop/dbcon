package dbcon

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/bool64/sqluct"
)

// resolveWorkPath resolves a relative path against the process working directory,
// rejecting absolute paths and any attempt to escape it.
func resolveWorkPath(rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", rel)
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	abs := filepath.Join(wd, rel)

	if abs != wd && !strings.HasPrefix(abs, wd+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes working directory: %s", rel)
	}

	return abs, nil
}

// csvCell renders a result value as a CSV cell, JSON-encoding non-string values.
func csvCell(v any) (string, error) {
	j, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	return strings.Trim(string(j), `"`), nil
}

// writeCSVRows writes columns and values as CSV, JSON-encoding non-string values.
func writeCSVRows(w *csv.Writer, cols []string, values [][]any) error {
	if err := w.Write(cols); err != nil {
		return err
	}

	rec := make([]string, len(cols))

	for _, row := range values {
		for i, v := range row {
			c, err := csvCell(v)
			if err != nil {
				return err
			}

			rec[i] = c
		}

		if err := w.Write(rec); err != nil {
			return err
		}
	}

	w.Flush()

	return w.Error()
}

// dialectStyle returns the placeholder format and identifier quoter for a dialect.
func dialectStyle(d sqluct.Dialect) (squirrel.PlaceholderFormat, func(...string) string) {
	switch d { //nolint:exhaustive
	case sqluct.DialectPostgres:
		return squirrel.Dollar, sqluct.QuoteANSI
	case sqluct.DialectMySQL, sqluct.DialectSQLite3:
		return squirrel.Question, sqluct.QuoteBackticks
	default: // duckdb and others.
		return squirrel.Question, sqluct.QuoteANSI
	}
}

// csvImportDirective is a parsed `-- csv:import=` directive.
type csvImportDirective struct {
	file   string // path or glob pattern
	table  string
	header bool
	cols   []string
	append bool
}

// isGlob reports whether s contains glob meta characters.
func isGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// parseCSVImport recognizes a `-- csv:import=<file|glob> [into=<table>] [header=false] [cols=a,b,c] [append=true]`
// directive statement.
//
// file may be a glob (e.g. `data_*.csv`) to load multiple uniform CSVs into one table; into= is
// then required, since there is no single file name to derive a table name from.
//
// header defaults to true (first CSV row names the columns, consumed as such). For a headerless
// file, either cols= must be given, or the target table must already exist so its columns can be
// reused. append=true inserts into an already existing table (e.g. to add another uniform CSV to
// a table a previous import created) instead of the default of skipping when the table exists,
// which keeps re-running the same statement safe.
func parseCSVImport(statement string) (d csvImportDirective, ok bool) {
	const prefix = "-- csv:import="

	line := strings.TrimSpace(statement)
	if !strings.HasPrefix(line, prefix) {
		return csvImportDirective{}, false
	}

	fields := strings.Fields(strings.TrimPrefix(line, prefix))
	if len(fields) == 0 {
		return csvImportDirective{}, false
	}

	d.file = fields[0]
	d.header = true

	if !isGlob(d.file) {
		d.table = strings.TrimSuffix(filepath.Base(d.file), filepath.Ext(d.file))
	}

	for _, f := range fields[1:] {
		switch {
		case strings.HasPrefix(f, "into="):
			d.table = strings.TrimPrefix(f, "into=")
		case strings.HasPrefix(f, "cols="):
			d.cols = strings.Split(strings.TrimPrefix(f, "cols="), ",")
		case f == "header=false":
			d.header = false
		case f == "append=true":
			d.append = true
		}
	}

	return d, true
}

// parseCSVExport finds a `-- csv:export=<file>` directive line in a statement.
func parseCSVExport(statement string) (file string, ok bool) {
	const prefix = "-- csv:export="

	for l := range strings.SplitSeq(statement, "\n") {
		l = strings.TrimSpace(l)
		if file, found := strings.CutPrefix(l, prefix); found {
			return strings.TrimSpace(file), true
		}
	}

	return "", false
}

// ImportCSV loads rows from a CSV file into table, optionally creating it first
// (all columns as TEXT). cols names the columns to insert into, and their order;
// if empty, the CSV header row is used (and consumed as such only when header is true).
func ImportCSV(ctx context.Context, db *sql.DB, dialect sqluct.Dialect, file, table string, header bool, cols []string, createTable bool) (int, error) {
	f, err := os.Open(file) //nolint:gosec
	if err != nil {
		return 0, err
	}

	defer func() {
		_ = f.Close() //nolint:errcheck
	}()

	r := csv.NewReader(f)

	if header {
		hdr, err := r.Read()
		if err != nil {
			return 0, err
		}

		if len(cols) == 0 {
			cols = hdr
		}
	}

	if len(cols) == 0 {
		return 0, errors.New("csv import: headerless file needs cols=, or an existing target table to borrow columns from")
	}

	format, quote := dialectStyle(dialect)
	quotedTable := quote(table)

	quotedCols := make([]string, len(cols))
	for i, c := range cols {
		quotedCols[i] = quote(c)
	}

	if createTable {
		createStmt := "CREATE TABLE " + quotedTable + " ("

		for _, c := range quotedCols {
			createStmt += c + " TEXT, "
		}

		createStmt = strings.TrimSuffix(createStmt, ", ") + ")"

		if _, err := db.ExecContext(ctx, createStmt); err != nil {
			return 0, err
		}
	}

	insert := "INSERT INTO " + quotedTable + " (" + strings.Join(quotedCols, ",") + ") VALUES (" +
		strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",") + ")"

	insert, err = format.ReplacePlaceholders(insert)
	if err != nil {
		return 0, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}

	args := make([]any, len(cols))
	count := 0
	batch := 0

	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return count, err
		}

		if len(record) != len(cols) {
			return count, fmt.Errorf("csv import: row %d has %d fields, expected %d", count+1, len(record), len(cols))
		}

		for i, v := range record {
			args[i] = v
		}

		if _, err := tx.ExecContext(ctx, insert, args...); err != nil {
			return count, err
		}

		count++
		batch++

		if batch >= 1000 {
			if err := tx.Commit(); err != nil {
				return count, err
			}

			batch = 0

			tx, err = db.BeginTx(ctx, nil)
			if err != nil {
				return count, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return count, err
	}

	return count, nil
}

// tableColumns reports whether quotedTable exists and, if so, its columns.
func tableColumns(ctx context.Context, db *sql.DB, quotedTable string) ([]string, bool) {
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+quotedTable+" LIMIT 0") //nolint:unqueryvet // probing for all columns of an unknown table is the point
	if err != nil {
		return nil, false
	}

	defer func() {
		_ = rows.Close() //nolint:errcheck
	}()

	cols, err := rows.Columns()
	if err != nil {
		return nil, false
	}

	if err := rows.Err(); err != nil {
		return nil, false
	}

	return cols, true
}

// tryCSVImport executes a `-- csv:import=` directive statement, if the statement is one.
//
// If the target table already exists, and the directive did not explicitly ask to reuse it
// (append=true, header=false or cols=), the import is skipped, so that re-running the same
// set of queries is safe. Otherwise rows are inserted into the existing table as-is. A glob
// pattern loads every match into the same table, the first one creating it if needed.
func tryCSVImport(ctx context.Context, db *sql.DB, dialect sqluct.Dialect, instanceName, statement string) (Result, bool) {
	d, ok := parseCSVImport(statement)
	if !ok {
		return Result{}, false
	}

	result := Result{Instance: instanceName, Statement: statement, Columns: []string{"csv_import"}}

	if isGlob(d.file) && d.table == "" {
		result.Error = "csv import: into= is required for a glob pattern"

		return result, true
	}

	pattern, err := resolveWorkPath(d.file)
	if err != nil {
		result.Error = err.Error()

		return result, true
	}

	files, err := filepath.Glob(pattern)
	if err != nil {
		result.Error = err.Error()

		return result, true
	}

	if len(files) == 0 {
		result.Error = fmt.Sprintf("csv import: no files match %s", d.file)

		return result, true
	}

	_, quote := dialectStyle(dialect)

	createTable := true

	if tableCols, exists := tableColumns(ctx, db, quote(d.table)); exists {
		createTable = false

		leverageExisting := d.append || !d.header || len(d.cols) > 0
		if !leverageExisting {
			result.Values = [][]any{{fmt.Sprintf("table %s already exists, skipped import", d.table)}}

			return result, true
		}

		if len(d.cols) == 0 {
			d.cols = tableCols
		}
	}

	total := 0

	for i, file := range files {
		n, err := ImportCSV(ctx, db, dialect, file, d.table, d.header, d.cols, createTable && i == 0)
		if err != nil {
			result.Error = fmt.Sprintf("%s: %s", file, err.Error())

			return result, true
		}

		total += n
	}

	msg := fmt.Sprintf("imported %d rows into %s", total, d.table)
	if len(files) > 1 {
		msg = fmt.Sprintf("imported %d rows from %d files into %s", total, len(files), d.table)
	}

	result.Values = [][]any{{msg}}

	return result, true
}
