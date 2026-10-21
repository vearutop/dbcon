// Package dbcon provides HTTP API.
package dbcon

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"log"

	"github.com/swaggest/rest/response"
	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
)

// DBQueryCSV returns query result as CSV.
func DBQueryCSV(deps Deps, options ...func(*Options)) usecase.Interactor {
	type request struct {
		Instance  instance `query:"instance"`
		Statement string   `query:"statement" formType:"textarea" title:"Statement" description:"SQL Statement to execute."`
	}

	u := usecase.NewInteractor(func(ctx context.Context, input request, output *response.EmbeddedSetter) error {
		var db *sql.DB

		for _, v := range deps.DBInstances() {
			if v.Name == string(input.Instance) {
				db = v.Instance

				break
			}
		}

		if db == nil {
			return status.Wrap(fmt.Errorf("unknown instance: %s", input.Instance), status.NotFound)
		}

		o := Options{}

		for _, opt := range options {
			opt(&o)
		}

		res := makeResult(ctx, db, string(input.Instance), input.Statement, o)

		if res.Error != "" {
			return errors.New(res.Error)
		}

		rw := output.ResponseWriter()
		rw.Header().Set("Content-Type", "text/csv")
		rw.Header().Set("Content-Disposition", "attachment; filename=\"data.csv\"")
		rw.Header().Set("Content-Transfer-Encoding", "binary")

		if err := writeCSVRows(csv.NewWriter(rw), res.Columns, res.Values); err != nil {
			log.Println("csv write failed:", err.Error())
		}

		return nil
	})

	return u
}
