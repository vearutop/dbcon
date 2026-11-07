package dbcon

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandTemplateVariables(t *testing.T) {
	for _, tt := range []struct {
		name, script, want string
	}{
		{
			name: "ordered setters and exact names",
			script: "SELECT $table;\n-- set:$table = first\nSELECT $table, $tableName;\n" +
				"-- set:$table=second\nSELECT $table;",
			want: "SELECT $table;\n\nSELECT first, $tableName;\n\nSELECT second;",
		},
		{
			name: "values with equals and references",
			script: "-- set:$filter=x = 1\n-- set:$condition=$filter AND y = '$unknown'\n" +
				"-- set:$unknown=known\nSELECT '$condition';",
			want: "\n\n\nSELECT 'x = 1 AND y = '$unknown'';",
		},
		{
			name:   "quoted identifiers and plot directives",
			script: "-- set:$tableName=be_enriched\n-- plot:caption=$tableName\nSELECT '$tableName', `$tableName` FROM $tableName;",
			want:   "\n-- plot:caption=be_enriched\nSELECT 'be_enriched', `be_enriched` FROM be_enriched;",
		},
		{
			name: "setter text in SQL is not a directive",
			script: "SELECT 'escaped '' quote\n-- set:$x=bad\n';\n/*\n-- set:$x=bad\n*/\n" +
				"SELECT 1; -- set:$x=bad\nSELECT $x;",
			want: "SELECT 'escaped '' quote\n-- set:$x=bad\n';\n/*\n-- set:$x=bad\n*/\n" +
				"SELECT 1; -- set:$x=bad\nSELECT $x;",
		},
		{
			name:   "empty value and final setter",
			script: "  -- set:$empty=  \r\nSELECT '$empty';\n-- set:$empty=last",
			want:   "  \nSELECT '';\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandTemplateVariables(tt.script)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestQueryInstance_TemplateVariables(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	_, err = db.ExecContext(ctx, "CREATE TABLE be_enriched (tracker TEXT, app TEXT); INSERT INTO be_enriched VALUES ('a', 'app1'), ('b', 'app1'), ('a', 'app2')")
	require.NoError(t, err)

	deps := dependencies{instances: []DBInstance{{Name: "test", Instance: db}}}
	query := dbQuery{Instance: "test", Statement: `-- set:$tableName=be_enriched
SELECT count(DISTINCT tracker) FROM $tableName;
-- plot:pie
SELECT count(1), app FROM $tableName GROUP BY app ORDER BY app;
-- set:$tableName=(SELECT 1)
SELECT count(1) FROM $tableName;
-- set:$unused=ignored`}
	results := queryInstance(ctx, deps, query, nil, Options{})
	require.Len(t, results, 3)

	for _, result := range results {
		require.Empty(t, result.Error)
		assert.NotContains(t, result.Statement, "$tableName")
	}

	assert.Equal(t, [][]any{{"2"}}, results[0].Values)
	assert.Equal(t, [][]any{{"2", "app1"}, {"1", "app2"}}, results[1].Values)
	assert.Contains(t, results[1].Statement, "-- plot:pie")
	assert.Equal(t, [][]any{{"1"}}, results[2].Values)

	// A separate execution must not inherit variables from the previous script.
	results = queryInstance(ctx, deps, dbQuery{Instance: "test", Statement: "SELECT '$tableName'"}, nil, Options{})
	require.Len(t, results, 1)
	require.Empty(t, results[0].Error)
	assert.Equal(t, [][]any{{"$tableName"}}, results[0].Values)
}

func TestTemplateLoops(t *testing.T) {
	for _, tt := range []struct {
		name, script string
		want         []string
	}{
		{
			name: "inline expressions and multiple statements",
			script: "-- set:$table=t\n-- for:$col in a, concat(b, coalesce(c, 'x,y')), `a,b`, 'it''s,a'\n" +
				"-- plot:pie\nSELECT count(1), $col FROM $table GROUP BY $col;\nSELECT $col FROM $table;\n-- endfor",
			want: []string{
				"-- plot:pie\nSELECT count(1), a FROM t GROUP BY a", "SELECT a FROM t",
				"-- plot:pie\nSELECT count(1), concat(b, coalesce(c, 'x,y')) FROM t GROUP BY concat(b, coalesce(c, 'x,y'))",
				"SELECT concat(b, coalesce(c, 'x,y')) FROM t",
				"-- plot:pie\nSELECT count(1), `a,b` FROM t GROUP BY `a,b`", "SELECT `a,b` FROM t",
				"-- plot:pie\nSELECT count(1), 'it''s,a' FROM t GROUP BY 'it''s,a'", "SELECT 'it''s,a' FROM t",
			},
		},
		{
			name: "multiline items with CRLF and indentation",
			script: "-- set:$col=outer\r\n  -- for:$col in\r\n  -- item:a\r\n  -- item:concat(b, c)\r\n" +
				"SELECT $col;\r\n  -- endfor\r\nSELECT '$col';",
			want: []string{"SELECT a", "SELECT concat(b, c)", "SELECT 'outer'"},
		},
		{
			name: "nested loops restore shadowed variable",
			script: "-- for:$x in 1, 2\nSELECT $x;\n-- for:$x in\n-- item:3\n-- item:4\n" +
				"SELECT $x;\n-- endfor\nSELECT $x;\n-- endfor\nSELECT '$x';",
			want: []string{"SELECT 1", "SELECT 3", "SELECT 4", "SELECT 1", "SELECT 2", "SELECT 3", "SELECT 4", "SELECT 2", "SELECT '$x'"},
		},
		{
			name: "setters and references in loop values",
			script: "-- set:$first=1\n-- for:$x in $first, 2\n-- set:$last=$x\n" +
				"SELECT $last;\n-- endfor\nSELECT $last;",
			want: []string{"SELECT 1", "SELECT 2", "SELECT 2"},
		},
		{
			name:   "loop markers inside quotes and comments are SQL text",
			script: "SELECT '\n-- for:$x in a\n-- endfor\n';\n/*\n-- endfor\n*/\nSELECT 1;",
			want:   []string{"SELECT '\n-- for:$x in a\n-- endfor\n'", "/*\n-- endfor\n*/\nSELECT 1"},
		},
		{
			name:   "quoted separators and commented parentheses",
			script: "-- for:$x in \"a,b\", `a``b,c`, f(1 /* , ) */, 2), ''\nSELECT $x;\n-- endfor",
			want:   []string{"SELECT \"a,b\"", "SELECT `a``b,c`", "SELECT f(1 /* , ) */, 2)", "SELECT ''"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			expanded, err := expandTemplateVariables(tt.script)
			require.NoError(t, err)
			assert.Equal(t, tt.want, SplitStatements(expanded))
		})
	}
}

func TestTemplateLoopErrors(t *testing.T) {
	for _, tt := range []struct{ script, message string }{
		{"-- for:$x in 1\nSELECT $x;", "line 1: for is missing endfor"},
		{"SELECT 1;\n-- endfor", "line 2: endfor without for"},
		{"-- for:$x in\nSELECT 1;\n-- endfor", "requires at least one item"},
		{"-- for:$x in 1,,2\n-- endfor", "empty for item"},
		{"-- for:$x in 1,\n-- endfor", "empty for item"},
		{"-- for:$x in f(1, 2\n-- endfor", "unbalanced parentheses"},
		{"-- for:$x in 1)\n-- endfor", "unbalanced parentheses"},
		{"-- for:$x in 'oops\n-- endfor", "unclosed quote"},
		{"-- for:$x in 'oops''\n-- endfor", "unclosed quote"},
		{"-- for:$x in 1 /*\n-- endfor", "unclosed comment"},
		{"-- for:x in 1\n-- endfor", "malformed loop directive"},
		{"-- for:$x in 1\n-- endfor extra", "malformed loop directive"},
		{"-- item:1", "item must appear at the start"},
		{"-- for:$x in\n-- item:\n-- endfor", "invalid item"},
		{"-- for:$x in\n-- item:f(1\n-- endfor", "invalid item"},
		{"-- for:$x in 1\nSELECT $x;\n-- item:2\n-- endfor", "item must appear at the start"},
	} {
		t.Run(tt.script, func(t *testing.T) {
			expanded, err := expandTemplateVariables(tt.script)
			require.ErrorContains(t, err, tt.message)
			assert.Empty(t, expanded)
		})
	}
}

func TestQueryInstance_TemplateLoops(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	_, err = db.ExecContext(ctx, "CREATE TABLE clicks (app TEXT, browser TEXT); INSERT INTO clicks VALUES ('a', 'x'), ('a', 'y'), ('b', 'x')")
	require.NoError(t, err)

	deps := dependencies{instances: []DBInstance{{Name: "test", Instance: db}}}

	results := queryInstance(ctx, deps, dbQuery{Instance: "test", Statement: `-- set:$table=clicks
-- for:$column in
-- item:app
-- item:browser
-- plot:pie
-- plot:caption=$column
SELECT count(1), $column FROM $table GROUP BY $column ORDER BY $column;
SELECT count(DISTINCT $column) FROM $table;
-- endfor`}, nil, Options{})
	require.Len(t, results, 4)

	for _, result := range results {
		require.Empty(t, result.Error)
		assert.NotContains(t, result.Statement, "$column")
	}

	assert.Equal(t, [][]any{{"2", "a"}, {"1", "b"}}, results[0].Values)
	assert.Contains(t, results[0].Statement, "-- plot:caption=app")
	assert.Equal(t, [][]any{{"2"}}, results[1].Values)
	assert.Equal(t, [][]any{{"2", "x"}, {"1", "y"}}, results[2].Values)
	assert.Contains(t, results[2].Statement, "-- plot:caption=browser")
	assert.Equal(t, [][]any{{"2"}}, results[3].Values)

	// Invalid templates must fail before even the first SQL statement executes.
	results = queryInstance(ctx, deps, dbQuery{Instance: "test", Statement: "INSERT INTO clicks VALUES ('c', 'z');\n-- for:$x in 1"}, nil, Options{})
	require.Len(t, results, 1)
	require.Contains(t, results[0].Error, "missing endfor")

	var count int

	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(1) FROM clicks").Scan(&count))
	assert.Equal(t, 3, count)
}
