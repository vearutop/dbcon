package gemini_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vearutop/dbcon/internal/llm/gemini"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestPrompter_Prompt(t *testing.T) {
	ctx := context.Background()

	t.Run("no auth key", func(t *testing.T) {
		p := &gemini.Prompter{}
		_, err := p.Prompt(ctx, "question")
		assert.EqualError(t, err, "no auth key")
	})

	t.Run("success", func(t *testing.T) {
		p := &gemini.Prompter{
			AuthKey: "key",
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(`{"candidates":[{"content":{"parts":[{"text":"  SELECT 1  "}]}}]}`), nil
			}),
		}

		answer, err := p.Prompt(ctx, "question")
		require.NoError(t, err)
		assert.Equal(t, "SELECT 1", answer)
	})

	t.Run("api error", func(t *testing.T) {
		p := &gemini.Prompter{
			AuthKey: "bad-key",
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(`{"error":{"code":400,"message":"API key not valid","status":"INVALID_ARGUMENT"}}`), nil
			}),
		}

		_, err := p.Prompt(ctx, "question")
		assert.EqualError(t, err, "API key not valid")
	})

	t.Run("blocked prompt", func(t *testing.T) {
		p := &gemini.Prompter{
			AuthKey: "key",
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(`{"promptFeedback":{"blockReason":"SAFETY"}}`), nil
			}),
		}

		_, err := p.Prompt(ctx, "question")
		assert.EqualError(t, err, "prompt blocked: SAFETY")
	})
}
