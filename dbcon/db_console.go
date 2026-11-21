package dbcon

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"io"
	"os"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/bool64/sqluct"
	"github.com/swaggest/rest/response"
	"github.com/swaggest/usecase"
	"github.com/swaggest/usecase/status"
	"github.com/vearutop/dbcon/internal/llm/gemini"
	"github.com/vearutop/dbcon/internal/llm/ollama"
	"github.com/vearutop/dbcon/internal/llm/openai"
	"github.com/vearutop/plot-go"
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
	DBInstances() []DBInstance
	Prompter() Prompter
}

// Prompter defines LLM service to ask questions about SQL.
type Prompter interface {
	Prompt(ctx context.Context, prompt string) (string, error)
}

type dependencies struct {
	instances []DBInstance
	prompter  Prompter
}

func (d dependencies) DBInstances() []DBInstance {
	return d.instances
}

func (d dependencies) Prompter() Prompter {
	return d.prompter
}

// loadCompletions reads schema of the instance, adds keywords and functions.
func (v DBInstance) loadCompletions(options ...func(o *Options)) (cmp []SQLCompletion, promptBase string) {
	switch v.Dialect { //nolint:exhaustive
	case sqluct.DialectSQLite3:
		return SqliteCompletions(v.Instance, options...)
	case sqluct.DialectPostgres:
		return PostgresCompletions(v.Instance, options...)
	case sqluct.DialectMySQL:
		return MySQLCompletions(v.Instance, options...)
	}

	return nil, ""
}

// PrepareInstances makes prepares DB instances for completions and UI.
func PrepareInstances(instances []DBInstance, options ...func(o *Options)) {
	for i, v := range instances {
		if v.prepared {
			continue
		}

		cmp, promptBase := v.loadCompletions(options...)
		v.Completions = append(v.Completions, cmp...)
		v.PromptBase = promptBase

		instances[i] = v
	}
}

// DefaultDeps prepares dependencies from DB instances.
func DefaultDeps(instances []DBInstance) Deps {
	deps := &dependencies{
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

	qr.fold()

	return qr, nil
}

// DecodeForm unpacks a form state, as used in the "form" URL parameter.
func DecodeForm(b string) (QueryRequest, error) {
	return decodeForm(b)
}

//go:embed console.html
var consoleHTML string

var consoleTemplate = template.Must(template.New("console").Parse(consoleHTML))

// DBConsole creates use case interactor to show DB console.
func DBConsole(deps Deps, prefix string, options ...func(*Options)) usecase.Interactor {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	type req struct {
		Form string `query:"form"`
	}

	o := Options{}
	for _, option := range options {
		option(&o)
	}

	u := usecase.NewInteractor(func(ctx context.Context, in req, out *response.EmbeddedSetter) error {
		qr, err := decodeForm(in.Form)
		if err != nil {
			return err
		}

		defaultInstance := ""
		if inst := findInstance(deps, ""); inst != nil {
			defaultInstance = inst.Name
		}

		// Completions are rebuilt per request (not cached at Mount time) so a schema refresh from any tab
		// is visible to every new page load, including ones opened from a shared link.
		out.ResponseWriter().Header().Set("Content-Type", "text/html; charset=utf-8")

		return consoleTemplate.Execute(out.ResponseWriter(), map[string]any{
			"Prefix":          prefix,
			"PlotHead":        template.HTML(plot.HeadTags(plotPrefix(prefix))), //nolint:gosec
			"Completions":     buildCompletions(deps, o),
			"DefaultInstance": defaultInstance,
			"Statement":       qr.Statement,
			"AutoRun":         in.Form != "",
			"AI":              deps.Prompter() != nil,
		})
	})

	u.SetExpectedErrors(status.Unknown, status.InvalidArgument)

	return u
}
