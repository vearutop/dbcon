package dbcon

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
	jsonform "github.com/swaggest/jsonform-go"
	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/rest/response"
	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
)

// Deps describes required resources.
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

// DefaultDeps prepares dependencies from DB instances.
func DefaultDeps(instances map[string]*sql.DB) Deps {
	return &dependencies{
		form:      jsonform.NewRepository(&jsonschema.Reflector{}),
		instances: instances,
	}
}

func decodeForm(b string) (qr QueryRequest, err error) {
	if len(b) == 0 {
		return qr, nil
	}

	var j []byte

	if b[0] != '{' {
		j, err = base64.StdEncoding.DecodeString(b)
		if err != nil {
			return qr, err
		}

		r := brotli.NewReader(bytes.NewReader(j))
		j, err = io.ReadAll(r)
		if err != nil {
			return qr, err
		}
	} else {
		j = []byte(b)
	}

	if err := json.Unmarshal(j, &qr); err != nil {
		return qr, err
	}

	return qr, nil
}

// DBConsole creates use case interactor to show DB console.
func DBConsole(deps Deps, prefix string) usecase.Interactor {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// PRAGMA table_info(visitor);

	type req struct {
		Form string `query:"form"`
	}

	u := usecase.NewInteractor(func(ctx context.Context, in req, out *response.EmbeddedSetter) error {
		p := jsonform.Page{}

		p.Title = "DB Console"

		p.AppendHTMLHead = template.HTML( //nolint:gosec
			`
<link rel="icon" href="` + prefix + `favicon.png" type="image/png"/>
<script src="` + prefix + `uPlot.iife.min.js"></script>
<script src="` + prefix + `script.js"></script>
<script src="` + prefix + `script_extra.js"></script>
<link rel="stylesheet" href="` + prefix + `style.css">
<link rel="stylesheet" href="` + prefix + `uPlot.min.css">
`)
		p.AppendHTML = `
<div style="margin: 2em">
<hr />
<div id="download-report" class="btn btn-info" style="display: none" onclick="downloadHTMLReport()">Download results as HTML report</div>
<a id="link-form" style="display: none" href="#">Link to this form</a>
<div id="query-results" style="margin-top:2em">
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

		qr, err := decodeForm(in.Form)
		if err != nil {
			return err
		}

		if len(qr.Queries) == 0 {
			qr.Queries = []dbQuery{{Instance: instance(instances)}}
		}

		return deps.SchemaRepository().Render(out.ResponseWriter(), p,
			jsonform.Form{
				Title:             "DB Console",
				SubmitURL:         prefix + "query-db",
				SubmitMethod:      http.MethodPost,
				SubmitText:        "Query",
				SuccessStatus:     http.StatusOK,
				Value:             qr,
				OnSuccess:         `onQuerySQLSuccess`,
				OnBeforeSubmit:    `onQuerySQLBeforeSubmit`,
				OnRequestFinished: `onQuerySQLFinished`,
			},
		)
	})

	u.SetExpectedErrors(status.Unknown, status.InvalidArgument)

	return u
}
