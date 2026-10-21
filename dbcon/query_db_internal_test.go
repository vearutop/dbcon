package dbcon

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // DB driver.
)

func TestMakeResult_CSVExport(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	_, err = db.ExecContext(ctx, "CREATE TABLE t (a INTEGER, b TEXT)")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO t VALUES (1,'x'),(2,'y')")
	require.NoError(t, err)

	out, err := os.CreateTemp(".", "csvtest-export-*.csv")
	require.NoError(t, err)
	require.NoError(t, out.Close())

	t.Cleanup(func() { _ = os.Remove(out.Name()) }) //nolint:errcheck

	//nolint:unqueryvet
	res := makeResult(ctx, db, "test", "SELECT * FROM t\n-- csv:export="+out.Name(), Options{})
	require.Empty(t, res.Error)

	// The result carries only a status message, not the buffered rows.
	assert.Equal(t, [][]any{{"streamed 2 rows to " + out.Name()}}, res.Values)

	content, err := os.ReadFile(out.Name())
	require.NoError(t, err)
	assert.Equal(t, "a,b\n1,x\n2,y\n", string(content))
}
