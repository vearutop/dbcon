package dbcon

import (
	"context"
	"database/sql"

	"github.com/bool64/sqluct"
)

// SqliteCompletions returns keywords, functions, tables and columns as code completions.
func SqliteCompletions(db *sql.DB, options ...func(o *Options)) (_ []SQLCompletion, promptBase string) { //nolint:maintidx
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
			tableNames[table] = true
		}
	}

	res := makeResult(context.Background(), db, "",
		"select tbl_name, sql from sqlite_master where type='table' and tbl_name != 'sqlite_sequence';", Options{})

	if len(res.Values) == 0 {
		return completions, ""
	}

	createTables := ""

	for _, row := range res.Values {
		table, ok := row[0].(string)
		if !ok {
			continue
		}

		if len(tableNames) > 0 && !tableNames[table] {
			continue
		}

		createTable, ok := row[1].(string)
		if !ok {
			continue
		}

		createTables += createTable + ";\n"

		completions = append(completions, SQLCompletion{
			Value: sqluct.QuoteRequiredBackticks(table),
			Score: 10000,
			Meta:  "table",
			Table: sqluct.QuoteRequiredBackticks(table),
		})

		tables = append(tables, table)
	}

	for _, table := range tables {
		if len(tableNames) > 0 && !tableNames[table] {
			continue
		}

		res = makeResult(context.Background(), db, "", "PRAGMA table_info("+sqluct.QuoteBackticks(table)+");", Options{})

		if len(res.Values) == 0 {
			continue
		}

		for _, row := range res.Values {
			if len(row) < 2 {
				continue
			}

			column, ok := row[1].(string)
			if !ok {
				continue
			}

			completions = append(completions, SQLCompletion{
				Value:  sqluct.QuoteRequiredBackticks(column),
				Score:  20000,
				Meta:   sqluct.QuoteRequiredBackticks(table),
				Table:  sqluct.QuoteRequiredBackticks(table),
				Column: sqluct.QuoteRequiredBackticks(column),
			})
		}
	}

	completions = addCompletionsFromStringList(`
    ABORT
    ACTION
    ADD
    AFTER
    ALL
    ALTER
    ALWAYS
    ANALYZE
    AND
    AS
    ASC
    ATTACH
    AUTOINCREMENT
    BEFORE
    BEGIN
    BETWEEN
    BY
    CASCADE
    CASE
    CAST
    CHECK
    COLLATE
    COLUMN
    COMMIT
    CONFLICT
    CONSTRAINT
    CREATE
    CROSS
    CURRENT
    CURRENT_DATE
    CURRENT_TIME
    CURRENT_TIMESTAMP
    DATABASE
    DEFAULT
    DEFERRABLE
    DEFERRED
    DELETE
    DESC
    DETACH
    DISTINCT
    DO
    DROP
    EACH
    ELSE
    END
    ESCAPE
    EXCEPT
    EXCLUDE
    EXCLUSIVE
    EXISTS
    EXPLAIN
    FAIL
    FILTER
    FIRST
    FOLLOWING
    FOR
    FOREIGN
    FROM
    FULL
    GENERATED
    GLOB
    GROUP
    GROUPS
    HAVING
    IF
    IGNORE
    IMMEDIATE
    IN
    INDEX
    INDEXED
    INITIALLY
    INNER
    INSERT
    INSTEAD
    INTERSECT
    INTO
    IS
    ISNULL
    JOIN
    KEY
    LAST
    LEFT
    LIKE
    LIMIT
    MATCH
    MATERIALIZED
    NATURAL
    NO
    NOT
    NOTHING
    NOTNULL
    NULL
    NULLS
    OF
    OFFSET
    ON
    OR
    ORDER
    OTHERS
    OUTER
    OVER
    PARTITION
    PLAN
    PRAGMA
    PRECEDING
    PRIMARY
    QUERY
    RAISE
    RANGE
    RECURSIVE
    REFERENCES
    REGEXP
    REINDEX
    RELEASE
    RENAME
    REPLACE
    RESTRICT
    RETURNING
    RIGHT
    ROLLBACK
    ROW
    ROWS
    SAVEPOINT
    SELECT
    SET
    TABLE
    TEMP
    TEMPORARY
    THEN
    TIES
    TO
    TRANSACTION
    TRIGGER
    UNBOUNDED
    UNION
    UNIQUE
    UPDATE
    USING
    VACUUM
    VALUES
    VIEW
    VIRTUAL
    WHEN
    WHERE
    WINDOW
    WITH
    WITHOUT`, "\n", "keyword", completions)

	completions = addCompletionsFromStringList(`
    abs(X)
    changes()
    char(X1,X2,...,XN)
    coalesce(X,Y,...)
    concat(X,...)
    concat_ws(SEP,X,...)
    format(FORMAT,...)
    glob(X,Y)
    hex(X)
    if(X,Y)
    if(X,Y,Z)
    ifnull(X,Y)
    iif(X,Y)
    iif(X,Y,Z)
    instr(X,Y)
    last_insert_rowid()
    length(X)
    like(X,Y)
    like(X,Y,Z)
    likelihood(X,Y)
    likely(X)
    load_extension(X)
    load_extension(X,Y)
    lower(X)
    ltrim(X)
    ltrim(X,Y)
    max(X,Y,...)
    min(X,Y,...)
    nullif(X,Y)
    octet_length(X)
    printf(FORMAT,...)
    quote(X)
    random()
    randomblob(N)
    replace(X,Y,Z)
    round(X)
    round(X,Y)
    rtrim(X)
    rtrim(X,Y)
    sign(X)
    soundex(X)
    sqlite_compileoption_get(N)
    sqlite_compileoption_used(X)
    sqlite_offset(X)
    sqlite_source_id()
    sqlite_version()
    substr(X,Y)
    substr(X,Y,Z)
    substring(X,Y)
    substring(X,Y,Z)
    total_changes()
    trim(X)
    trim(X,Y)
    typeof(X)
    unhex(X)
    unhex(X,Y)
    unicode(X)
    unlikely(X)
    upper(X)
    zeroblob(N)`, "\n", "core-func", completions)

	completions = addCompletionsFromStringList(`
    date(time-value, modifier, modifier, ...)
    time(time-value, modifier, modifier, ...)
    datetime(time-value, modifier, modifier, ...)
    julianday(time-value, modifier, modifier, ...)
    unixepoch(time-value, modifier, modifier, ...)
    strftime(format, time-value, modifier, modifier, ...)
    timediff(time-value, time-value)`, "\n", "date-func", completions)

	completions = addCompletionsFromStringList(`
    avg(X)
    count(*)
    count(X)
    group_concat(X)
    group_concat(X,Y)
    max(X)
    min(X)
    string_agg(X,Y)
    sum(X)
    total(X)`, "\n", "aggregate-func", completions)

	completions = addCompletionsFromStringList(`
	row_number()
	rank()
	dense_rank()
	percent_rank()
	cume_dist()
	ntile(N)
	lag(expr)
	lag(expr, offset)
	lag(expr, offset, default)
	lead(expr)
	lead(expr, offset)
	lead(expr, offset, default)
	first_value(expr)
	last_value(expr)
	nth_value(expr, N)`, "\n", "window-func", completions)

	completions = addCompletionsFromStringList(`
    acos(X)
    acosh(X)
    asin(X)
    asinh(X)
    atan(X)
    atan2(Y,X)
    atanh(X)
    ceil(X)
    ceiling(X)
    cos(X)
    cosh(X)
    degrees(X)
    exp(X)
    floor(X)
    ln(X)
    log(B,X)
    log(X)
    log10(X)
    log2(X)
    mod(X,Y)
    pi()
    pow(X,Y)
    power(X,Y)
    radians(X)
    sin(X)
    sinh(X)
    sqrt(X)
    tan(X)
    tanh(X)
    trunc(X)`, "\n", "math-func", completions)

	completions = addCompletionsFromStringList(`
    json(json)
    jsonb(json)
    json_array(value1,value2,...)
    jsonb_array(value1,value2,...)
    json_array_length(json)
    json_array_length(json,path)
    json_error_position(json)
    json_extract(json,path,...)
    jsonb_extract(json,path,...)
    json -> path
    json ->> path
    json_insert(json,path,value,...)
    jsonb_insert(json,path,value,...)
    json_object(label1,value1,...)
    jsonb_object(label1,value1,...)
    json_patch(json1,json2)
    jsonb_patch(json1,json2)
    json_pretty(json)
    json_remove(json,path,...)
    jsonb_remove(json,path,...)
    json_replace(json,path,value,...)
    jsonb_replace(json,path,value,...)
    json_set(json,path,value,...)
    jsonb_set(json,path,value,...)
    json_type(json)
    json_type(json,path)
    json_valid(json)
    json_valid(json,flags)
    json_quote(value)`, "\n", "json-scalar-func", completions)

	completions = addCompletionsFromStringList(`
    json_group_array(value)
    jsonb_group_array(value)
    json_group_object(label,value)
    jsonb_group_object(name,value)`, "\n", "json-aggregate-func", completions)

	completions = addCompletionsFromStringList(`
    json_each(json)
    json_each(json,path)
    json_tree(json)
    json_tree(json,path)`, "\n", "json-func", completions)

	promptBase = "Given the following SQLite database schema, answer my next question with SQL statement, " +
		"only use columns defined in the schema:\n " +
		createTables + "\n\n"

	return completions, promptBase
}
