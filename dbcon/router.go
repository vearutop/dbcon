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
func Mount(s *web.Service, prefix string, deps Deps) {
	s.Get("/", DBConsole(deps, prefix))
	s.Get("/db.html", DBConsole(deps, prefix))
	s.Post("/query-db", DBQuery(deps))
	s.Get("/query-db.csv", DBQueryCSV(deps))

	s.Mount("/", http.StripPrefix(prefix, staticServer))
}

// Handler creates an HTTP handler.
func Handler(prefix string, deps Deps) http.Handler {
	s := web.NewService(openapi31.NewReflector())

	Mount(s, prefix, deps)

	return s
}
