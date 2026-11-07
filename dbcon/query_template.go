package dbcon

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	templateVariable      = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)
	templateSetter        = regexp.MustCompile(`^--\s*set:\s*(\$[A-Za-z_][A-Za-z0-9_]*)\s*=(.*)$`)
	templateFor           = regexp.MustCompile(`^--\s*for:\s*(\$[A-Za-z_][A-Za-z0-9_]*)\s+in(?:\s+(.*))?$`)
	templateItem          = regexp.MustCompile(`^--\s*item:\s*(.*)$`)
	templateEndFor        = regexp.MustCompile(`^--\s*endfor\s*$`)
	templateLoopDirective = regexp.MustCompile(`^--\s*(?:for:|item:|endfor\b)`)
)

type templateNode struct {
	text     string
	name     string
	values   []string
	children []*templateNode
	line     int
}

// expandTemplateVariables validates the whole script before expanding setters and
// loops. Replacement is a single pass of raw text, including quotes and comments.
func expandTemplateVariables(script string) (string, error) {
	root, err := parseQueryTemplate(script)
	if err != nil {
		return "", err
	}

	var out strings.Builder

	expandTemplateNodes(&out, root.children, map[string]string{})

	return out.String(), nil
}

func replaceTemplateVariables(text string, vars map[string]string) string {
	return templateVariable.ReplaceAllStringFunc(text, func(name string) string {
		if value, ok := vars[name]; ok {
			return value
		}

		return name
	})
}

func expandTemplateNodes(out *strings.Builder, nodes []*templateNode, vars map[string]string) {
	for _, node := range nodes {
		switch {
		case node.values != nil:
			previous, existed := vars[node.name]
			values := make([]string, len(node.values))

			for i, value := range node.values {
				values[i] = replaceTemplateVariables(value, vars)
			}

			for _, value := range values {
				vars[node.name] = value
				expandTemplateNodes(out, node.children, vars)
			}

			if existed {
				vars[node.name] = previous
			} else {
				delete(vars, node.name)
			}
		case node.name != "":
			vars[node.name] = replaceTemplateVariables(node.text, vars)
		default:
			out.WriteString(replaceTemplateVariables(node.text, vars))
		}
	}
}

func parseQueryTemplate(script string) (*templateNode, error) {
	root := &templateNode{}
	stack := []*templateNode{root}
	textStart := 0

	for i := 0; i < len(script); {
		start := i
		i = templateSQLChunkEnd(script, i)

		if !strings.HasPrefix(script[start:], "--") {
			continue
		}

		lineStart := strings.LastIndexByte(script[:start], '\n') + 1
		if strings.TrimSpace(script[lineStart:start]) != "" {
			continue
		}

		comment := strings.TrimSpace(script[start:i])
		setter := templateSetter.FindStringSubmatch(comment)

		if setter == nil && !templateLoopDirective.MatchString(comment) {
			continue
		}

		parent := stack[len(stack)-1]
		parent.children = append(parent.children, &templateNode{text: script[textStart:start]})
		textStart = i
		line := strings.Count(script[:start], "\n") + 1

		switch {
		case setter != nil:
			parent.children = append(parent.children, &templateNode{name: setter[1], text: strings.TrimSpace(setter[2])})
		case templateFor.MatchString(comment):
			m := templateFor.FindStringSubmatch(comment)
			values := []string{}

			if strings.TrimSpace(m[2]) != "" {
				items, err := splitTemplateItems(m[2])
				if err != nil {
					return nil, fmt.Errorf("template line %d: %w", line, err)
				}

				values = items
			}

			loop := &templateNode{name: m[1], values: values, line: line}
			parent.children = append(parent.children, loop)
			stack = append(stack, loop)
		case templateItem.MatchString(comment):
			if len(stack) == 1 || templateBodyStarted(parent.children) {
				return nil, fmt.Errorf("template line %d: item must appear at the start of a for block", line)
			}

			value := strings.TrimSpace(templateItem.FindStringSubmatch(comment)[1])
			if _, err := splitTemplateItems(value); err != nil {
				return nil, fmt.Errorf("template line %d: invalid item: %w", line, err)
			}

			parent.values = append(parent.values, value)
		case templateEndFor.MatchString(comment):
			if len(stack) == 1 {
				return nil, fmt.Errorf("template line %d: endfor without for", line)
			}

			if len(parent.values) == 0 {
				return nil, fmt.Errorf("template line %d: for requires at least one item", parent.line)
			}

			stack = stack[:len(stack)-1]
		default:
			return nil, fmt.Errorf("template line %d: malformed loop directive", line)
		}
	}

	if len(stack) != 1 {
		return nil, fmt.Errorf("template line %d: for is missing endfor", stack[len(stack)-1].line)
	}

	root.children = append(root.children, &templateNode{text: script[textStart:]})

	return root, nil
}

func templateBodyStarted(nodes []*templateNode) bool {
	for _, node := range nodes {
		if node.name != "" || strings.TrimSpace(node.text) != "" {
			return true
		}
	}

	return false
}

// splitTemplateItems separates commas outside quotes, comments and parentheses.
func splitTemplateItems(list string) ([]string, error) {
	var items []string

	depth, start := 0, 0

	for i := 0; i < len(list); {
		c := list[i]
		if strings.ContainsRune("'\"`", rune(c)) {
			end, closed := templateQuoteEnd(list, i)
			if !closed {
				return nil, fmt.Errorf("unclosed quote in for items")
			}

			i = end

			continue
		}

		if strings.HasPrefix(list[i:], "--") || strings.HasPrefix(list[i:], "/*") {
			end := templateSQLChunkEnd(list, i)
			if strings.HasPrefix(list[i:], "/*") && !strings.HasSuffix(list[i:end], "*/") {
				return nil, fmt.Errorf("unclosed comment in for items")
			}

			i = end

			continue
		}

		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced parentheses in for items")
			}
		case ',':
			if depth == 0 {
				items = append(items, strings.TrimSpace(list[start:i]))
				start = i + 1
			}
		}

		i++
	}

	if depth != 0 {
		return nil, fmt.Errorf("unbalanced parentheses in for items")
	}

	items = append(items, strings.TrimSpace(list[start:]))
	for _, item := range items {
		if item == "" {
			return nil, fmt.Errorf("empty for item")
		}
	}

	return items, nil
}

func templateQuoteEnd(script string, i int) (int, bool) {
	quote := script[i]
	i++

	for i < len(script) {
		if script[i] == quote {
			i++
			if i < len(script) && script[i] == quote {
				i++

				continue
			}

			return i, true
		}

		i++
	}

	return i, false
}

// templateSQLChunkEnd skips a SQL quote, comment or run of ordinary text.
func templateSQLChunkEnd(script string, i int) int {
	switch {
	case strings.HasPrefix(script[i:], "--"):
		if end := strings.IndexByte(script[i:], '\n'); end >= 0 {
			return i + end
		}

		return len(script)
	case strings.HasPrefix(script[i:], "/*"):
		if end := strings.Index(script[i+2:], "*/"); end >= 0 {
			return i + end + 4
		}

		return len(script)
	case strings.ContainsRune("'\"`", rune(script[i])):
		end, _ := templateQuoteEnd(script, i)

		return end
	default:
		i++
		for i < len(script) && !strings.ContainsRune("-'\"`/", rune(script[i])) {
			i++
		}

		return i
	}
}
