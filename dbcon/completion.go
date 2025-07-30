package dbcon

import (
	"strings"
)

// SQLCompletion describes code completion unit.
type SQLCompletion struct {
	Value  string `json:"value" example:"foo.bar"`
	Score  int    `json:"score" example:"1000"`
	Meta   string `json:"meta" example:"column"`
	Table  string `json:"table,omitempty"`
	Column string `json:"column,omitempty"`
}

func addCompletionsFromStringList(list string, sep string, meta string, completions []SQLCompletion) []SQLCompletion {
	for _, t := range strings.Split(list, sep) {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}

		// Add function completion without arguments.
		if strings.Contains(t, "(") {
			completions = append(completions, SQLCompletion{
				Value: strings.SplitN(t, "(", 2)[0] + "(",
				Score: 1000,
				Meta:  meta,
			})
		}

		completions = append(completions, SQLCompletion{
			Value: t,
			Score: 1000,
			Meta:  meta,
		})
	}

	return completions
}
