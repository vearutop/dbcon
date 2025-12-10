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
	"os"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/bool64/sqluct"
	jsonform "github.com/swaggest/jsonform-go"
	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/rest/response"
	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
	"github.com/vearutop/dbcon/internal/llm/gemini"
	"github.com/vearutop/dbcon/internal/llm/ollama"
	"github.com/vearutop/dbcon/internal/llm/openai"
)

// Options defines DBCon customizable params.
type Options struct {
	Completions []SQLCompletion

	// TableNames limits the tables to use in completions, all tables are used if empty.
	TableNames []string

	// valueProcessor is map of function name to processor function.
	valueProcessor map[string]func(any) any
}

// AddValueProcessor adds a function to postprocess result column value.
func (o *Options) AddValueProcessor(name string, fn func(any) any) {
	if o.valueProcessor == nil {
		o.valueProcessor = make(map[string]func(any) any)
	}

	o.valueProcessor[name] = fn
}

// DBInstance describes a DB instance.
type DBInstance struct {
	Name        string
	Dialect     sqluct.Dialect
	Instance    *sql.DB
	Completions []SQLCompletion
	PromptBase  string // Prompt with available tables and columns.

	prepared bool
}

// Deps defines required resources.
type Deps interface {
	SchemaRepository() *jsonform.Repository
	DBInstances() []DBInstance
	Prompter() Prompter
}

// Prompter defines LLM service to ask questions about SQL.
type Prompter interface {
	Prompt(ctx context.Context, prompt string) (string, error)
}

type dependencies struct {
	form      *jsonform.Repository
	instances []DBInstance
	prompter  Prompter
}

func (d dependencies) SchemaRepository() *jsonform.Repository {
	return d.form
}

func (d dependencies) DBInstances() []DBInstance {
	return d.instances
}

func (d dependencies) Prompter() Prompter {
	return d.prompter
}

// PrepareInstances makes prepares DB instances for completions and UI.
func PrepareInstances(instances []DBInstance, options ...func(o *Options)) {
	instancesEnum = nil

	for i, v := range instances {
		instancesEnum = append(instancesEnum, v.Name)

		if v.prepared {
			continue
		}

		switch v.Dialect { //nolint:exhaustive
		case sqluct.DialectSQLite3:
			cmp, promptBase := SqliteCompletions(v.Instance, options...)
			v.Completions = append(v.Completions, cmp...)
			v.PromptBase = promptBase
		case sqluct.DialectPostgres:
			cmp, promptBase := PostgresCompletions(v.Instance, options...)
			v.Completions = append(v.Completions, cmp...)
			v.PromptBase = promptBase
		}

		instances[i] = v
	}
}

// DefaultDeps prepares dependencies from DB instances.
func DefaultDeps(instances []DBInstance) Deps {
	deps := &dependencies{
		form:      jsonform.NewRepository(&jsonschema.Reflector{}),
		instances: instances,
	}

	if ollamaModel := os.Getenv("DBCON_OLLAMA_MODEL"); ollamaModel != "" {
		deps.prompter = &ollama.Prompter{Model: ollamaModel}
	} else if authKey := os.Getenv("DBCON_GEMINI_API_KEY"); authKey != "" {
		deps.prompter = &gemini.Prompter{AuthKey: authKey, ModelName: os.Getenv("DBCON_GEMINI_MODEL")}
	} else if authKey := os.Getenv("DBCON_OPENAI_KEY"); authKey != "" {
		deps.prompter = &openai.Prompter{AuthKey: authKey}
	}

	return deps
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
func DBConsole(deps Deps, prefix string, options ...func(*Options)) usecase.Interactor {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// PRAGMA table_info(visitor);

	type req struct {
		Form string `query:"form"`
	}

	o := Options{}
	for _, option := range options {
		option(&o)
	}

	completions := map[string][]SQLCompletion{}
	cmp := []SQLCompletion{
		{Value: "-- plot:cols", Score: 1000, Meta: "exp cols: x, y1, y2, ..."},
		{Value: "-- plot:rows", Score: 1000, Meta: "exp cols: x, y, label"},
		{Value: "-- plot:time", Score: 1000, Meta: "time series"},
		{Value: "-- pie", Score: 1000, Meta: "exp cols: count, label"},
		{Value: "-- pie:total=X", Score: 1000, Meta: "override total count"},
		{Value: "-- strip", Score: 1000, Meta: "render data only"},
		{Value: "-- # ", Score: 1000, Meta: "add header"},
		{Value: "-- > ", Score: 1000, Meta: "add description"},
	}

	cmp = append(cmp, o.Completions...)

	for k := range o.valueProcessor {
		cmp = append(cmp, SQLCompletion{Value: "-- " + k + ":column_name", Score: 1000, Meta: "value processor"})
	}

	for _, v := range deps.DBInstances() {
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

		qr, err := decodeForm(in.Form)
		if err != nil {
			return err
		}

		if in.Form != "" {
			p.AppendHTML += `<script>$(function(){$('#schema-form-queries').submit();})</script>`
		}

		if len(qr.Queries) == 0 {
			qr.Queries = []dbQuery{{}}
		}

		queriesForm := jsonform.Form{
			Name:              "queries",
			Title:             "DB Console",
			SubmitURL:         prefix + "query-db",
			SubmitMethod:      http.MethodPost,
			SubmitText:        "Query",
			SuccessStatus:     http.StatusOK,
			Value:             qr,
			OnSuccess:         `onQuerySQLSuccess`,
			OnBeforeSubmit:    `onQuerySQLBeforeSubmit`,
			OnRequestFinished: `onQuerySQLFinished`,
		}

		askAIForm := jsonform.Form{
			Name:              "ask-ai",
			SubmitURL:         prefix + "prompt",
			SubmitMethod:      http.MethodPost,
			SubmitText:        "Send",
			Value:             promptRequest{},
			OnSuccess:         `onPromptSuccess`,
			OnBeforeSubmit:    `onPromptBeforeSubmit`,
			OnRequestFinished: `onPromptFinished`,
			BeforeForm:        `<div id="columns-directory" class="pure-u-2-5" style="position: absolute"></div><div style="position: absolute;margin-left: 160px" class="ai btn btn-info" onclick="return toggleAskAI();">Ask AI 🤖</div>`,
		}

		if deps.Prompter() == nil {
			p.AppendHTMLHead += `<style>.ai { display: none }</style>
`
		}

		return deps.SchemaRepository().Render(out.ResponseWriter(), p, queriesForm, askAIForm)
	})

	u.SetExpectedErrors(status.Unknown, status.InvalidArgument)

	return u
}
