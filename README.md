# dbcon

[![Build Status](https://github.com/vearutop/dbcon/workflows/test-unit/badge.svg)](https://github.com/vearutop/dbcon/actions?query=branch%3Amaster+workflow%3Atest-unit)
[![Coverage Status](https://codecov.io/gh/vearutop/dbcon/branch/master/graph/badge.svg)](https://codecov.io/gh/vearutop/dbcon)
[![GoDevDoc](https://img.shields.io/badge/dev-doc-00ADD8?logo=go)](https://pkg.go.dev/github.com/vearutop/dbcon)
[![Time Tracker](https://wakatime.com/badge/github/vearutop/dbcon.svg)](https://wakatime.com/badge/github/vearutop/dbcon)
![Code lines](https://sloc.xyz/github/vearutop/dbcon/?category=code)
![Comments](https://sloc.xyz/github/vearutop/dbcon/?category=comments)

Web-based SQL console to SQLite, MySQL and Postgres.

## Install

### Macos Brew

```
brew tap vearutop/tools && brew update && brew install dbcon
```

### Go Install

```
go install github.com/vearutop/dbcon@latest
$(go env GOPATH)/bin/dbcon --help
```

Or download binary from [releases](https://github.com/vearutop/dbcon/releases).

### Linux AMD64

```
wget https://github.com/vearutop/dbcon/releases/latest/download/linux_amd64.tar.gz && tar xf linux_amd64.tar.gz && rm linux_amd64.tar.gz
./dbcon -version
```

### Macos Intel

```
wget https://github.com/vearutop/dbcon/releases/latest/download/darwin_amd64.tar.gz && tar xf darwin_amd64.tar.gz && rm darwin_amd64.tar.gz
codesign -s - ./dbcon
./dbcon -version
```

### Macos Apple Silicon (M1, etc...)

```
wget https://github.com/vearutop/dbcon/releases/latest/download/darwin_arm64.tar.gz && tar xf darwin_arm64.tar.gz && rm darwin_arm64.tar.gz
codesign -s - ./dbcon
./dbcon -version
```


## Usage

```
Usage of dbcon:
dbcon [OPTIONS] DB...
        DB can be a path to SQLite/CSV/JSONL file, or a URL with mysql:// or postgres:// scheme. Examples:
                postgres://user:password@localhost/dbname?sslmode=disable
                mysql://user:password@localhost/dbname
                sqlite:///my.db
                my.sqlite
                my2.csv
                my3.jsonl
                my4.log
  -auth string
        basic auth as user:password
  -decode-form string
        print SQL statements of an encoded form (value of the form URL parameter, or a full URL) and exit
  -listen string
        listen address, port 0 picks a free random port (default "127.0.0.1:0")
  -prefix string
        URL path prefix, random by default so that the port alone does not give access, "/" to serve from the root
  -s    skip browser opening
  -sql string
        SQL script file to open in the console and run on start
  -tables string
        comma-separated list table names to use for completion and AI
```

The console is served under a random URL path prefix (for example `http://127.0.0.1:54321/5k3m0qz9x1b7e/`), which is printed on start and opened in the browser, so a port alone is not enough to reach it. Requests outside the prefix get 404. Use `-prefix` to set a stable prefix (e.g. behind a reverse proxy), or `-prefix /` to serve from the root. The prefix is a convenience against port scanning, not a replacement for `-auth`: it travels in URLs (and so in browser history and shared links).

Multiple statements can be separated with `;`.

With several databases (`dbcon a.sqlite b.sqlite`), statements run on the first one by default. A standalone
`-- instance:b.sqlite` comment (available in completions) switches the instance for its statement and all following
ones, until the next `-- instance:` comment.

The editor completes SQL in context (SQLite, Postgres, MySQL): tables after `FROM`/`JOIN`, columns of the tables
in scope (aliases, CTEs and derived tables included) elsewhere, columns of an alias after `alias.`, qualified names
where a column is ambiguous, and `-- ` directives in comments. Matching is fuzzy, so a fragment of a long name
(`click.Track`, `ctr`) finds `.context.click.CallbackData.Tracker`.

Define template variables with a standalone `-- set:$name=value` comment:

```sql
-- set:$tableName=be_enriched
select count(DISTINCT `.context.click.CallbackData.Tracker`) from $tableName;

-- plot:pie
select count(1), `.datl.appName`
from $tableName
group by `.datl.appName`;
```

Names are case-sensitive and use letters, digits, and underscores (starting with a letter or underscore).
Setters apply from their position onward within the same query script; setting a name again changes its value for subsequent text.
Values are trimmed and inserted as raw text, including inside SQL quotes and other comment directives; include any SQL quoting in the value as needed.
Values may reference previously set variables. Unknown variables remain unchanged, and replacements are not recursively expanded.
Setter comments do not produce query results and do not need a semicolon.

Repeat a block of statements with `-- for:$name in ...` and `-- endfor`:

```sql
-- set:$clicksTable=good_enr
-- for:$column in `.datl.appName`, `.datl.id`, concat(`.datl.browserName`, `.datl.browserVersion`)
-- plot:pie
-- plot:caption=$column
select count(1), $column from $clicksTable group by $column;
select count(DISTINCT $column) from $clicksTable;
-- endfor
```

For longer lists, put each expression on its own `-- item:` line immediately after the loop header:

```sql
-- for:$column in
-- item:`.datl.appName`
-- item:`.datl.id`
-- item:`.datl.browserName`
-- item:concat(`.datl.browserName`, `.datl.browserVersion`)

-- plot:pie
select count(1), $column from $clicksTable group by $column;
-- endfor
```

Inline lists split on commas outside quotes, comments, and parentheses. Each `-- item:` line supplies one complete expression.
All statements in the block run for the first item, then for the next, producing separate results and charts.
Terminate SQL statements with semicolons as usual. Loop directives must be standalone comments.
Loops can nest. The loop variable is restored to its previous value after the block; other setters retain their usual effect on following text.
Item values may reference existing variables and are expanded when entering the loop.
Malformed loops report a template error with a line number before any SQL in that query script executes.

If a statement has `-- plot` in comment, result is plotted on a chart.

First column is used for X axis, remaining columns go to Y axis.

X-axis can be a UTC datetime if values are UNIX timestamp integers and statement has a comment `-- plot:time`.

Use `-- plot:x.format={YYYY}-{MM}` to override time axis label format (default is `{HH}:{mm}\n{YYYY}-{MM}-{DD}`).

Use `-- plot:caption=My dataset` to set a plot caption.

Use `-- plot:height=360px` to override plot height.

Use `-- plot:x.unit=s` / `-- plot:y.unit=s` when values of that axis are seconds, to show them as Go-style durations (`5d23h12m5s`) on the axis (and in the legend for X). `x.unit` is ignored with `-- plot:time`. For histograms and heatmaps use `-- plot:value.unit=s`, whichever axis the value is on.

Plot settings are `-- plot:path=value` lines with a dotted path into [plot-go](https://github.com/vearutop/plot-go)'s options (`x.type`, `x.unit`, `x.format`, `y.unit`, `value.unit`, `pie.total`, ...); `caption` and `height` are bare keys. Short directives choose the chart: `-- plot:time` (same as `x.type=time`), `-- plot:heatmap`, `-- plot:pie`, `-- plot:stacked_bars`, `-- plot:rows`. Unknown paths are passed through and ignored by the chart.

For stacked bars, add `-- plot:stacked_bars`. Combine with `-- plot:rows` for `x, y, label` shape or omit for `x, y1, y2...`.

If a statement has `-- plot:pie` in comment, result is rendered as a pie chart.

First column is used for the numeric value of pie slice, second is a label.

Pie total is calculated as sum of all values, for cases of partial pie you can provide the total with `-- plot:pie.total=123`.

## Histograms

`-- histogram` combines numeric values into adaptive buckets using [dynhist-go](https://github.com/vearutop/dynhist-go).
Each input row is `(count, value)`: the value contributes `count` observations. For example, collect URL lengths:

```sql
-- histogram:buckets=20
-- plot:stacked_bars
SELECT count(1) cnt, length(url) l FROM log GROUP BY l;
```

The equivalent raw input is `SELECT 1, length(url) l FROM log;` with the same directives.
Use your database's string-length function (`length`, `strlen`, etc.).
Row order and aggregation can affect adaptive bucket boundaries, while weighted totals remain the same.

Without a plot directive, results are a table of `min`, `max`, `count`, and `percent`.
With any `-- plot` directive (a bare `-- plot` is enough), each bar represents a bucket; labels show its observed range and tooltips show its count and percentage of all collected observations.
Bars are equally spaced categories, even when numeric ranges have different widths.
To stack series, add a third column with the series label and use `-- plot:rows` (same as the `x, y, label` shape of other plots): rows are `(count, value, series)`, all series are binned into the same `buckets` ranges (taken from the whole result), and the result rows are `series, min, max, count, percent`, where percent is of all observations, so the stack sums to 100%. Series are stacked in the order of appearance, the legend toggles them. Select the series column by name with `-- histogram:series=col`.

```sql
-- histogram:buckets=30
-- plot:rows
-- plot:value.unit=s
SELECT count(1), round(install_time) t, channel FROM installs GROUP BY t, channel;
```

With `-- plot:cols` and three or more columns the input is wide instead: the first column is the value and each other column is the count of a series named by the column (`SELECT t, sum(channel = 'ads') ads, sum(channel = 'organic') organic FROM installs GROUP BY t`). With two columns `-- plot:rows` and `-- plot:cols` change nothing. Percentiles are calculated over all series.

`-- plot:heatmap` (with `-- histogram`) expects `count, value, x` columns and builds one histogram per distinct `x`, all binned into the same `buckets` ranges (taken from the whole result). Result rows are `x, min, max, count, percent`, where percent is relative to the `x` slice. `x` is sorted numerically when all values are numbers, otherwise kept in first-seen order; select it by name with `-- histogram:x=col`. Cells are colored by percent of the slice (with `-- histogram:percent`) or by count; value buckets are drawn as equal-height rows, so `distribution=exp` or `latency` spreads them like a log scale. `-- plot:time` formats numeric `x` as UTC time; `-- plot:x.unit=s` formats the `x` axis and `-- plot:value.unit=s` the value axis (and percentiles) as durations.

`-- plot:value.unit=s` treats histogram values as seconds and shows bucket ranges and percentiles as Go-style durations (`5d23h12m5s`, `1.5s`, `300ms`).

`-- plot:caption=...` and `-- plot:height=...` also apply.

Options can be combined on one directive or supplied on separate histogram directive lines:

* `-- histogram:buckets=20`: maximum number of output buckets (default 20, allowed 1–10000).
* `-- histogram:buckets=all`: one bucket per distinct value, for discrete values such as hours or counters (up to 10000 buckets, if there are more distinct values the closest ones are merged). Percentiles are exact in this mode.
* `-- histogram:buckets=20 collect_buckets=200`: retain up to 200 buckets while scanning, then merge adjacent buckets down to at most 20 for display and CSV export. This keeps finer detail during collection with bounded memory. `collect_buckets` defaults to 200 (or `buckets` if higher), must be at least `buckets`, and supports the same 1–10000 range. Final merging uses the same smallest combined range strategy as collection and preserves counts and sums.
* `-- histogram:distribution=latency`: bucket width strategy, one of `linear` (default, equal-width buckets), `latency` (narrow buckets for low values, wider for high) or `exp` (exponentially growing, dynhist `ExpWidth`; tune with `sum_width_pow` (default 1.2, raise to widen low-value buckets) and `spacing_pow` (default 1, raise to widen high-value buckets), both numbers greater than 0, only used by `exp`). Applies to both collection and final merging.
* `-- histogram:count=cnt value=l`: select columns by exact alias, overriding the default first and second columns.
* `-- histogram:percent`: display percentages instead of counts on the chart's Y axis.
* `-- histogram:percentiles=50,90,95,99`: show percentile estimates above the chart or table. Values must be greater than 0 and at most 100; fractional percentiles such as `99.9` are supported. Estimates use dynhist's batch `Percentiles` API and nearest-rank selection on the weighted collection buckets **before squashing**, and use the same units as the value column. Hover an estimate to see its collection bucket range. Empty distributions have no percentile estimates.

For example, show response-time percentiles alongside the distribution:

```sql
-- histogram:percentiles=50,90,95,99
-- plot:stacked_bars
SELECT 1, response_time_ms FROM log;
```

Percentile summaries also appear in portable HTML reports. CSV exports continue to contain only the histogram bucket rows.

Counts must be nonnegative integers. Numeric strings are accepted; nonnumeric and nonfinite values report an error.
Zero-weight rows and NULL values are skipped. Percentages use the total weight of the remaining observations.
Collection streams rows and retains only the buckets, without expanding weighted rows into individual observations.
Histogram directives work inside template loops. CSV downloads and `-- csv:export=...` export the histogram buckets.

## CSV Import/Export

`-- csv:import=<file|glob>` as a statement of its own imports a CSV file into a table on the
selected instance, all as `TEXT` columns. Paths are resolved relative to dbcon's working
directory (absolute paths and `..` are rejected).

* By default the table name comes from the file name, and the first CSV row is used as header. Override with `into=table_name`.
* `header=false` treats every row as data. Combine with `cols=a,b,c` to name the columns, or omit `cols=` to reuse the columns of an already existing table.
* If the target table already exists, the import is skipped (so re-running the same query is safe), unless `header=false`, `cols=`, or `append=true` is given, in which case rows are inserted into it.
* A glob, e.g. `-- csv:import=data_*.csv into=my_table`, loads every match into the same table (`into=` is then required).

`-- csv:export=<file>` on a `SELECT` statement streams its result straight into a CSV file in dbcon's working directory, instead of returning it to the console (which reports only the row count once done). Rows are written as they come off the DB driver, without buffering the full result in memory, so this also works for selections too large to hold in memory or ship back as JSON.

## AI Assistance

LLM can help to translate a query in natural language into SQL. You can enable LLM support by configuring credentials
in environment variables.

* `DBCON_OPENAI_KEY=<API_KEY>` for [ChatGPT](https://platform.openai.com/api-keys)
* `DBCON_GEMINI_API_KEY=<API_KEY>` for [Google Gemini](https://ai.google.dev/gemini-api/docs/api-key)
* `DBCON_OLLAMA_MODEL=codegemma:7b` for local [Ollama model](https://ollama.com/library/codegemma:7b)

![Example](dbcon.png)
