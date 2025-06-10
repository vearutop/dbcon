package dbcon

import (
	"embed"
	"net/http"

	"github.com/swaggest/openapi-go/openapi31"
	"github.com/swaggest/rest/web"
	"github.com/vearutop/statigz"
)

var (
	// staticAssets holds embedded static assets.
	//
	//go:embed static/*
	staticAssets embed.FS

	staticServer = statigz.FileServer(staticAssets, statigz.FSPrefix("static"))
)

// Mount instruments the router with handlers.
func Mount(s *web.Service, prefix string, deps Deps, options ...func(*Options)) {
	s.Get("/", DBConsole(deps, prefix, options...))
	s.Get("/db.html", DBConsole(deps, prefix, options...))
	s.Post("/query-db", DBQuery(deps, options...))
	s.Get("/query-db.csv", DBQueryCSV(deps, options...))
	s.Post("/prompt", Prompt(deps))

	s.Mount("/", http.StripPrefix(prefix, staticServer))
	deps.SchemaRepository().Mount(s, "/json-form/")
}

// Handler creates an HTTP handler.
func Handler(prefix string, deps Deps, options ...func(*Options)) http.Handler {
	s := web.NewService(openapi31.NewReflector())

	Mount(s, prefix, deps, options...)

	return s
}
