package dbcon

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/bool64/sqluct"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // DB driver.
)

func TestParseCSVImport(t *testing.T) {
	d, ok := parseCSVImport("-- csv:import=./data.csv")
	require.True(t, ok)
	assert.Equal(t, csvImportDirective{file: "./data.csv", table: "data", header: true}, d)

	d, ok = parseCSVImport("-- csv:import=./data.csv into=t1 header=false cols=a,b append=true")
	require.True(t, ok)
	assert.Equal(t, csvImportDirective{file: "./data.csv", table: "t1", header: false, cols: []string{"a", "b"}, append: true}, d)

	d, ok = parseCSVImport("-- csv:import=data_*.csv into=t2")
	require.True(t, ok)
	assert.Equal(t, csvImportDirective{file: "data_*.csv", table: "t2", header: true}, d)

	_, ok = parseCSVImport("SELECT 1")
	assert.False(t, ok)
}

func TestParseCSVExport(t *testing.T) {
	file, ok := parseCSVExport("SELECT 1\n-- csv:export=out.csv")
	require.True(t, ok)
	assert.Equal(t, "out.csv", file)

	_, ok = parseCSVExport("SELECT 1")
	assert.False(t, ok)
}

func TestResolveWorkPath(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	abs, err := resolveWorkPath("data.csv")
	require.NoError(t, err)
	assert.Equal(t, wd+"/data.csv", abs)

	_, err = resolveWorkPath("/etc/passwd")
	assert.Error(t, err)

	_, err = resolveWorkPath("../secret.csv")
	assert.Error(t, err)
}

func TestTryCSVImport(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	f1, err := os.CreateTemp(".", "csvtest-a-*.csv")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(f1.Name()) })
	_, err = f1.WriteString("a,b\n1,x\n2,y\n")
	require.NoError(t, err)
	require.NoError(t, f1.Close())

	f2, err := os.CreateTemp(".", "csvtest-b-*.csv")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(f2.Name()) })
	_, err = f2.WriteString("a,b\n3,z\n4,w\n")
	require.NoError(t, err)
	require.NoError(t, f2.Close())

	pattern := "csvtest-*.csv"

	res, ok := tryCSVImport(ctx, db, sqluct.DialectSQLite3, "test", "-- csv:import="+pattern+" into=t")
	require.True(t, ok)
	require.Empty(t, res.Error)
	assert.Equal(t, [][]any{{"imported 4 rows from 2 files into t"}}, res.Values)

	rows, err := db.QueryContext(ctx, "SELECT count(*) FROM t")
	require.NoError(t, err)

	var n int
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&n))
	require.NoError(t, rows.Close())
	assert.Equal(t, 4, n)

	// Re-running the same import is a no-op by default.
	res, ok = tryCSVImport(ctx, db, sqluct.DialectSQLite3, "test", "-- csv:import="+pattern+" into=t")
	require.True(t, ok)
	assert.Contains(t, res.Values[0][0], "skipped")

	// append=true inserts into the already existing table instead of skipping.
	res, ok = tryCSVImport(ctx, db, sqluct.DialectSQLite3, "test", "-- csv:import="+f1.Name()+" into=t append=true")
	require.True(t, ok)
	require.Empty(t, res.Error)

	rows, err = db.QueryContext(ctx, "SELECT count(*) FROM t")
	require.NoError(t, err)

	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&n))
	require.NoError(t, rows.Close())
	assert.Equal(t, 6, n)
}
