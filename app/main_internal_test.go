package app

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/url"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestURLPrefix(t *testing.T) {
	for in, want := range map[string]string{"/": "/", "abc": "/abc/", "/a/b": "/a/b/", "/a/": "/a/"} {
		got, err := urlPrefix(in)
		require.NoError(t, err)
		assert.Equal(t, want, got, in)
	}

	a, err := urlPrefix("")
	require.NoError(t, err)

	b, err := urlPrefix("")
	require.NoError(t, err)

	assert.Regexp(t, `^/[0-9a-z]{13}/$`, a)
	assert.NotEqual(t, a, b)
}

func TestWithSQL(t *testing.T) {
	t.Parallel()

	u, err := withSQL("http://h/p/", "select 1; -- ü")
	require.NoError(t, err)

	form, err := url.Parse(u)
	require.NoError(t, err)

	b, err := base64.StdEncoding.DecodeString(form.Query().Get("form"))
	require.NoError(t, err)

	j, err := io.ReadAll(brotli.NewReader(bytes.NewReader(b)))
	require.NoError(t, err)
	assert.JSONEq(t, `{"statement":"select 1; -- ü"}`, string(j))
}
