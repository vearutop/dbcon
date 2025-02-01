package dbcon

import (
	"strings"
)

// SQLCompletion describes code completion unit.
type SQLCompletion struct {
	Value string `json:"value" example:"foo.bar"`
	Score int    `json:"score" example:"1000"`
	Meta  string `json:"meta" example:"column"`
}

func addCompletionsFromStringList(list string, meta string, completions []SQLCompletion) []SQLCompletion {
	for _, t := range strings.Split(list, "\n") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}

		completions = append(completions, SQLCompletion{
			Value: t,
			Score: 100,
			Meta:  meta,
		})
	}

	return completions
}
