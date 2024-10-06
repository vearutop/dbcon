# dbcon

[![Build Status](https://github.com/vearutop/dbcon/workflows/test-unit/badge.svg)](https://github.com/vearutop/dbcon/actions?query=branch%3Amaster+workflow%3Atest-unit)
[![Coverage Status](https://codecov.io/gh/vearutop/dbcon/branch/master/graph/badge.svg)](https://codecov.io/gh/vearutop/dbcon)
[![GoDevDoc](https://img.shields.io/badge/dev-doc-00ADD8?logo=go)](https://pkg.go.dev/github.com/vearutop/dbcon)
[![Time Tracker](https://wakatime.com/badge/github/vearutop/dbcon.svg)](https://wakatime.com/badge/github/vearutop/dbcon)
![Code lines](https://sloc.xyz/github/vearutop/dbcon/?category=code)
![Comments](https://sloc.xyz/github/vearutop/dbcon/?category=comments)

Web-based SQL console to SQLite, MySQL and Postgres.

## Install

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
        DB can be a path to SQLite file, or a URL with mysql:// or postgres:// scheme. Examples:
                postgres://user:password@localhost/dbname?sslmode=disable
                mysql://user:password@localhost/dbname
                sqlite:///my.db
                my.sqlite
  -listen string
        listen address, port 0 picks a free random port (default "127.0.0.1:0")
```