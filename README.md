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
        DB can be a path to SQLite/CSV file, or a URL with mysql:// or postgres:// scheme. Examples:
                postgres://user:password@localhost/dbname?sslmode=disable
                mysql://user:password@localhost/dbname
                sqlite:///my.db
                my.sqlite
                my2.csv
  -listen string
        listen address, port 0 picks a free random port (default "127.0.0.1:0")
  -s    skip browser opening
```

Multiple statements can be separated with `;`.

If a statement has `-- plot` in comment, result is plotted on a chart.

First column is used for X axis, remaining columns go to Y axis.

X-axis can be a UTC datetime if values are UNIX timestamp integers and statement has a comment `-- plot:time`.

If a statement has `-- pie` in comment, result is rendered as a pie chart.

First column is used for the numeric value of pie slice, second is a label.

Pie total is calculated as sum of all values, for cases of partial pie you can provide the total with `-- pie:total=123`.

## AI Assistance

LLM can help to translate a query in natural language into SQL. You can enable LLM support by configuring credentials
in environment variables.

* `DBCON_OPENAI_KEY=<API_KEY>` for [ChatGPT](https://platform.openai.com/api-keys)
* `DBCON_GEMINI_API_KEY=<API_KEY>` for [Google Gemini](https://ai.google.dev/gemini-api/docs/api-key)
* `DBCON_OLLAMA_MODEL=codegemma:7b` for local [Ollama model](https://ollama.com/library/codegemma:7b)

![Example](dbcon.png)
