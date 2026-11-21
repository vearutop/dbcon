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

// addGenericCompletions adds common ANSI keywords and functions, for dialects without a dedicated list.
func addGenericCompletions(completions []SQLCompletion) []SQLCompletion {
	completions = addCompletionsFromStringList(strings.ToUpper(`select|insert|update|delete|from|where|and|or|group|by|order|limit|offset|having|as|case|when|then|else|end|type|left|right|join|on|outer|desc|asc|union|create|table|primary|key|if|foreign|not|references|default|null|inner|cross|natural|database|drop|grant|distinct|is|in|all|alter|any|array|at|authorization|between|both|cast|check|collate|column|commit|constraint|cube|current|current_date|current_time|current_timestamp|current_user|describe|escape|except|exists|external|extract|fetch|filter|for|full|function|global|grouping|intersect|interval|into|leading|like|local|no|of|only|out|overlaps|partition|position|range|revoke|rollback|rollup|row|rows|session_user|set|some|start|tablesample|time|to|trailing|truncate|unique|unknown|user|using|values|window|with`),
		"|", "keyword", completions)

	completions = addCompletionsFromStringList(strings.ToUpper(`avg|count|first|last|max|min|sum|ucase|lcase|mid|len|round|rank|now|format|coalesce|ifnull|isnull|nvl`),
		"|", "function", completions)

	return completions
}
