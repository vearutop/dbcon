package dbcon

import (
	"embed"
	"net/http"
	"strings"

	"github.com/swaggest/openapi-go/openapi31"
	"github.com/swaggest/rest/web"
	"github.com/vearutop/plot-go"
	"github.com/vearutop/statigz"
)

var (
	// staticAssets holds embedded static assets.
	//
	//go:embed static/*
	staticAssets embed.FS

	staticServer = statigz.FileServer(staticAssets, statigz.FSPrefix("static"))
)

// Mount instruments the router with handlers. All routes are served under the path prefix, "/" for the root.
func Mount(s *web.Service, prefix string, deps Deps, options ...func(*Options)) {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	s.Get(prefix, DBConsole(deps, prefix, options...))
	s.Get(prefix+"db.html", DBConsole(deps, prefix, options...))
	s.Post(prefix+"query-db", DBQuery(deps, options...))
	s.Get(prefix+"query-db.csv", DBQueryCSV(deps, options...))
	s.Post(prefix+"prompt", Prompt(deps))
	s.Post(prefix+"refresh-schema", RefreshSchema(deps, options...))

	s.Mount(prefix, http.StripPrefix(prefix, staticServer))
	s.Mount(plotPrefix(prefix), http.StripPrefix(plotPrefix(prefix), plot.Handler()))
	deps.SchemaRepository().Mount(s, prefix+"json-form/")
}

// plotPrefix is where the plot package's static assets (uPlot, chart rendering script) are
// served from, so DBConsole can point HeadTags at the same path.
func plotPrefix(prefix string) string {
	return strings.TrimSuffix(prefix, "/") + "/plot/"
}

// Handler creates an HTTP handler.
func Handler(prefix string, deps Deps, options ...func(*Options)) http.Handler {
	s := web.NewService(openapi31.NewReflector())

	Mount(s, prefix, deps, options...)

	return s
}
