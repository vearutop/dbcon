package dbcon_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bool64/sqluct"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vearutop/dbcon/dbcon"
	_ "modernc.org/sqlite" // DB driver.
)

func TestHandler_prefix(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	instances := []dbcon.DBInstance{{Name: "test", Dialect: sqluct.DialectSQLite3, Instance: db}}
	dbcon.PrepareInstances(instances)

	h := dbcon.Handler("/secret/", dbcon.DefaultDeps(instances))

	do := func(method, path string) int {
		rw := httptest.NewRecorder()
		h.ServeHTTP(rw, httptest.NewRequest(method, path, strings.NewReader("")))

		return rw.Code
	}

	// Everything is served under the prefix.
	for _, path := range []string{"/secret/", "/secret/db.html", "/secret/script.js", "/secret/plot/plot.js", "/secret/jquery-3.7.1.min.js"} {
		assert.Equal(t, http.StatusOK, do(http.MethodGet, path), path)
	}

	// And nothing without it.
	for _, path := range []string{"/", "/db.html", "/script.js", "/plot/plot.js", "/jquery-3.7.1.min.js", "/query-db.csv"} {
		assert.Equal(t, http.StatusNotFound, do(http.MethodGet, path), path)
	}

	for _, path := range []string{"/query-db", "/prompt", "/refresh-schema"} {
		assert.Equal(t, http.StatusNotFound, do(http.MethodPost, path), path)
	}

	assert.Equal(t, http.StatusOK, do(http.MethodPost, "/secret/refresh-schema"))
}
