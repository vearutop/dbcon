package app

import (
	"testing"

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
