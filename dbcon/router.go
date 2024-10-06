package dbcon

import (
	"embed"
	"net/http"

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

func Mount(s *web.Service, prefix string, deps Deps) {
	s.Get("/db.html", DBConsole(deps, prefix))
	s.Post("/query-db", DBQuery(deps))
	s.Get("/query-db.csv", DBQueryCSV(deps))

	s.Mount(prefix, http.StripPrefix(prefix, staticServer))

	deps.SchemaRepository().Mount(s, "/json-form/")
}
