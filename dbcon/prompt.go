package dbcon

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/swaggest/usecase"
	"github.com/yuin/goldmark"
)

type promptRequest struct {
	Instance instance `json:"instance" title:"DB Instance"`
	Question string   `json:"question" formType:"textarea" title:"Question" description:"Ask question about data in natural language, receive SQL statement."`
}

// Prompt creates usecase interactor to prompt LLM about SQL.
func Prompt(deps Deps) usecase.Interactor {
	type response struct {
		Message string `json:"message"`
	}

	md := goldmark.New()

	u := usecase.NewInteractor(func(ctx context.Context, input promptRequest, output *response) error {
		prompter := deps.Prompter()

		if prompter == nil {
			return errors.New("no prompter")
		}

		promptBase := ""

		for _, ins := range deps.DBInstances() {
			if ins.Name == string(input.Instance) {
				promptBase = ins.PromptBase
			}
		}

		if promptBase == "" {
			return errors.New("no prompt base")
		}

		prompt := promptBase + input.Question

		answer, err := prompter.Prompt(ctx, prompt)
		if err != nil {
			return err
		}

		if strings.Contains(answer, "```") {
			var buf bytes.Buffer
			if err := md.Convert([]byte(answer), &buf); err != nil {
				return err
			}

			output.Message = buf.String()
		} else {
			output.Message = "<pre>" + answer + "</pre>"
		}

		return nil
	})

	return u
}
