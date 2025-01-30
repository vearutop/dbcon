package dbcon

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/swaggest/usecase"
)

type dbQuery struct {
	Instance  instance `json:"instance" title:"DB Instance"`
	Statement string   `json:"statement" formType:"textarea" title:"SQL Statements"`
}

type instance string

func (i instance) Enum() (res []any) {
	if len(i) > 0 {
		instances := strings.Split(string(i), ",")
		for _, instance := range instances {
			res = append(res, instance)
		}
	}

	return res
}

// QueryRequest is a list of queries.
type QueryRequest struct {
	Queries []dbQuery `json:"queries" title:"Queries" description:"SQL statements to execute."`
}

// Result is an SQL statement query result.
type Result struct {
	Statement string          `json:"statement"`
	Columns   []string        `json:"columns"`
	Values    [][]interface{} `json:"values"`
	Elapsed   string          `json:"elapsed"`
	Error     string          `json:"error,omitempty"`
	Instance  string          `json:"instance"`
}

// Response is an envelope for Result items and extra shared information.
type Response struct {
	Results []Result `json:"results,omitempty"`
	Form    string   `json:"form,omitempty" description:"Base64 encoded brotli compressed incoming JSON request for a form param."`
}

// DBQuery queries SQL statements and returns results as JSON.
func DBQuery(deps Deps) usecase.Interactor {
	u := usecase.NewInteractor(func(ctx context.Context, input QueryRequest, output *Response) error {
		var results []Result

		for _, q := range input.Queries {
			results = queryInstance(ctx, deps, q, results)
		}

		output.Results = results

		j, err := json.Marshal(input)
		if err != nil {
			return err
		}

		buf := bytes.NewBuffer(nil)
		w := brotli.NewWriter(buf)

		if _, err := w.Write(j); err != nil {
			return err
		}

		if err := w.Close(); err != nil {
			return err
		}

		output.Form = base64.StdEncoding.EncodeToString(buf.Bytes())

		return nil
	})

	return u
}

func queryInstance(ctx context.Context, deps Deps, query dbQuery, results []Result) []Result {
	db := deps.DBInstances()[string(query.Instance)]

	if db == nil {
		result := Result{
			Instance: string(query.Instance),
			Error:    fmt.Sprintf("unknown instance: %s", query.Instance),
		}
		results = append(results, result)

		return results
	}

	for _, statement := range SplitStatements(query.Statement) {
		results = append(results, makeResult(ctx, db, string(query.Instance), statement))
	}

	return results
}

func makeResult(ctx context.Context, db *sql.DB, instance, statement string) (result Result) {
	result = Result{
		Instance: instance,
	}

	statement = strings.TrimSpace(statement)
	result.Statement = statement

	start := time.Now()

	rows, err := db.QueryContext(ctx, statement)
	if err != nil {
		result.Error = err.Error()

		return result
	}

	result.Elapsed = time.Since(start).String()

	cols, err := rows.Columns()
	if err != nil {
		result.Error = err.Error()

		return result
	}

	defer func() {
		if err := rows.Err(); err != nil {
			result.Error += fmt.Sprint(" rows error:", err.Error())
		}

		if err := rows.Close(); err != nil {
			result.Error += fmt.Sprint(" rows close:", err.Error())
		}
	}()

	result.Columns = cols

	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePointers := make([]interface{}, len(cols))

		for i := range values {
			valuePointers[i] = &values[i]
		}

		if err := rows.Scan(valuePointers...); err != nil {
			result.Error = "scan rows: " + err.Error()

			return result
		}

		for i, v := range values {
			if iv, ok := v.(int64); ok {
				values[i] = strconv.Itoa(int(iv))
			}
		}

		result.Values = append(result.Values, values)
	}

	return result
}
