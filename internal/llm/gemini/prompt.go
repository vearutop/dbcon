// Package gemini implements LLM client for Google Gemini.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// https://ai.google.dev/gemini-api/docs/vision?lang=rest&authuser=1

/*
curl "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=GEMINI_API_KEY" \
-H 'Content-Type: application/json' \
-X POST \
-d '{
  "contents": [{
    "parts":[{"text": "Explain how AI works"}]
    }]
   }'
*/

/*
IMG_PATH=/path/to/your/image1.jpeg

if [[ "$(base64 --version 2>&1)" = *"FreeBSD"* ]]; then
  B64FLAGS="--input"
else
  B64FLAGS="-w0"
fi

curl "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=$GOOGLE_API_KEY" \
    -H 'Content-Type: application/json' \
    -X POST \
    -d '{
      "contents": [{
        "parts":[
            {"text": "Caption this image."},
            {
              "inline_data": {
                "mime_type":"image/jpeg",
                "data": "'\$(base64 \$B64FLAGS \$IMG_PATH)'"
              }
            }
        ]
      }]
    }' 2> /dev/null
*/

// Prompter can ask LLM about DB.
type Prompter struct {
	AuthKey   string
	Transport http.RoundTripper // default http.DefaultTransport.
	ModelName string
}

// Response describes Gemini response.
type Response struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason     string `json:"finishReason"`
		CitationMetadata struct {
			CitationSources []struct {
				StartIndex int `json:"startIndex"`
				EndIndex   int `json:"endIndex"`
			} `json:"citationSources"`
		} `json:"citationMetadata"`
		AvgLogprobs float64 `json:"avgLogprobs"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
		PromptTokensDetails  []struct {
			Modality   string `json:"modality"`
			TokenCount int    `json:"tokenCount"`
		} `json:"promptTokensDetails"`
		CandidatesTokensDetails []struct {
			Modality   string `json:"modality"`
			TokenCount int    `json:"tokenCount"`
		} `json:"candidatesTokensDetails"`
	} `json:"usageMetadata"`
	ModelVersion string `json:"modelVersion"`
}

// Model returns the name of LLM.
func (ip *Prompter) Model() string {
	if ip.ModelName != "" {
		return ip.ModelName
	}

	return "gemini-2.5-flash"
}

// Prompt asks LLM to translate a question to SQL statement.
func (ip *Prompter) Prompt(ctx context.Context, prompt string) (string, error) {
	if ip.AuthKey == "" {
		return "", errors.New("no auth key")
	}

	type Part struct {
		Text string `json:"text,omitempty"`
	}

	type Content struct {
		Parts []Part `json:"parts"`
	}

	type Req struct {
		Contents []Content `json:"contents"`
	}

	req := Req{}
	req.Contents = []Content{
		{
			Parts: []Part{
				{Text: prompt},
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	r, err := http.NewRequestWithContext(ctx,
		http.MethodPost,
		"https://generativelanguage.googleapis.com/v1beta/models/"+ip.Model()+":generateContent?key="+ip.AuthKey,
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	r.Header.Set("Content-Type", "application/json")

	tr := ip.Transport
	if tr == nil {
		tr = http.DefaultTransport
	}

	resp, err := tr.RoundTrip(r)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close() //nolint:errcheck

	cont, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	re := Response{}

	if err := json.Unmarshal(cont, &re); err != nil {
		return "", err
	}

	if len(re.Candidates) == 0 {
		return "", errors.New("no candidates found")
	}

	c := re.Candidates[0]
	if len(c.Content.Parts) == 0 {
		return "", errors.New("no parts found")
	}

	return strings.Trim(c.Content.Parts[0].Text, "\" \t\n"), nil
}
