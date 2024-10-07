package dbcon

import (
	"context"
	"database/sql"
	"html/template"
	"net/http"
	"strings"

	jsonform "github.com/swaggest/jsonform-go"
	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/rest/response"
	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
)

type Deps interface {
	SchemaRepository() *jsonform.Repository
	DBInstances() map[string]*sql.DB
}

type dependencies struct {
	form      *jsonform.Repository
	instances map[string]*sql.DB
}

func (d dependencies) SchemaRepository() *jsonform.Repository {
	return d.form
}

func (d dependencies) DBInstances() map[string]*sql.DB {
	return d.instances
}

func DefaultDeps(instances map[string]*sql.DB) Deps {
	return &dependencies{
		form:      jsonform.NewRepository(&jsonschema.Reflector{}),
		instances: instances,
	}
}

// DBConsole creates use case interactor to show DB console.
func DBConsole(deps Deps, prefix string) usecase.Interactor {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	u := usecase.NewInteractor(func(ctx context.Context, in struct{}, out *response.EmbeddedSetter) error {
		p := jsonform.Page{}

		p.Title = "DB Console"
		p.AppendHTMLHead = template.HTML(`
<link rel="icon" href="` + prefix + `favicon.png" type="image/png"/>
<script src="` + prefix + `jquery-3.7.1.slim.min.js"></script>
<script src="` + prefix + `script.js"></script>
<link rel="stylesheet" href="` + prefix + `/style.css">
`)
		p.AppendHTML = `
<div style="margin: 2em">

<a href="#" style="display:none;margin-bottom: 10px" id="dl-csv" class="btn btn-primary" target="_blank">Download CSV</a> <span id="num-rows"></span>
<div id="query-results">

</div>
</div>
`

		instances := ""
		for k := range deps.DBInstances() {
			instances += "," + k
		}

		if instances != "" {
			instances = instances[1:]
		}

		return deps.SchemaRepository().Render(out.ResponseWriter(), p,
			jsonform.Form{
				Title:             "DB Console",
				SubmitURL:         prefix + "query-db",
				SubmitMethod:      http.MethodPost,
				SuccessStatus:     http.StatusOK,
				Value:             dbQuery{Instance: instance(instances)},
				OnSuccess:         `onQuerySQLSuccess`,
				OnBeforeSubmit:    `onQuerySQLBeforeSubmit`,
				OnRequestFinished: `onQuerySQLFinished`,
			},
		)
	})

	u.SetExpectedErrors(status.Unknown, status.InvalidArgument)

	return u
}
