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
	"github.com/bool64/sqluct"
	jsonform "github.com/swaggest/jsonform-go"
	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/rest/response"
	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
)

// DBInstance describes a DB instance.
type DBInstance struct {
	Name        string
	Dialect     sqluct.Dialect
	Instance    *sql.DB
	Completions []SQLCompletion
}

// Deps describes required resources.
type Deps interface {
	SchemaRepository() *jsonform.Repository
	DBInstances() []DBInstance
}

type dependencies struct {
	form      *jsonform.Repository
	instances []DBInstance
}

func (d dependencies) SchemaRepository() *jsonform.Repository {
	return d.form
}

func (d dependencies) DBInstances() []DBInstance {
	return d.instances
}

// DefaultDeps prepares dependencies from DB instances.
func DefaultDeps(instances []DBInstance) Deps {
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

	completions := map[string][]SQLCompletion{}
	cmp := []SQLCompletion{
		{Value: "-- plot:cols", Score: 1000, Meta: "from cols: X, Y1, Y2, ..."},
		{Value: "-- plot:rows", Score: 1000, Meta: "from rows: X, Y, Label"},
		{Value: "-- plot:time", Score: 1000, Meta: "time series"},
		{Value: "-- pie", Score: 1000, Meta: "draw pie chart"},
		{Value: "-- pie:total=X", Score: 1000, Meta: "draw pie chart"},
	}

	for _, v := range deps.DBInstances() {
		switch v.Dialect { //nolint:exhaustive
		case sqluct.DialectSQLite3:
			v.Completions = append(v.Completions, SqliteCompletions(v.Instance)...)
		case sqluct.DialectPostgres:
			v.Completions = append(v.Completions, PostgresCompletions(v.Instance)...)
		}

		completions[v.Name] = append(v.Completions, cmp...)
	}

	u := usecase.NewInteractor(func(ctx context.Context, in req, out *response.EmbeddedSetter) error {
		p := jsonform.Page{}

		p.Title = "DB Console"

		j, err := json.Marshal(completions)
		if err != nil {
			return err
		}

		p.AppendHTMLHead = template.HTML( //nolint:gosec
			`
<script src="` + prefix + `ace/ace.min.js"></script>
<script src="` + prefix + `ace/ext-inline_autocomplete.min.js"></script>
<script src="` + prefix + `ace/ext-language_tools.min.js"></script>

<link rel="icon" href="` + prefix + `favicon.png" type="image/png"/>
<script src="` + prefix + `uPlot.iife.min.js"></script>
<script src="` + prefix + `script.js"></script>
<script src="` + prefix + `script_extra.js"></script>
<link rel="stylesheet" href="` + prefix + `style.css">
<link rel="stylesheet" href="` + prefix + `uPlot.min.css">

<script>
completions = ` + string(j) + `
</script>
`)
		p.AppendHTML = `
<div style="margin: 2em">
<hr />
<script>
renderColumnsDirectory();
</script>

<div id="download-report" class="btn btn-info" style="display: none" onclick="downloadHTMLReport()">Download results as HTML report</div>
<a id="link-form" style="display: none" href="#">Link to this form</a>
<div id="query-results" style="margin-top:2em">
</div>
</div>
`

		instances := ""
		for _, v := range deps.DBInstances() {
			instances += "," + v.Name
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
