package variant

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/mumoshu/variant2/pkg/app"
	variantslack "github.com/mumoshu/variant2/pkg/slack"
	"github.com/nlopes/slack"
	"github.com/zclconf/go-cty/cty"
)

func (r *Runner) StartSlackbot(name string) error {
	// (This is written by calling isatty beforehand and this just overwrite that
	r.Interactive = true

	bot := variantslack.New(name, os.Getenv("SLACK_BOT_TOKEN"), os.Getenv("SLACK_VERIFICATION_TOKEN"), func(bot *variantslack.Connection, cmd string, message slack.SlashCommand) string {
		var b bytes.Buffer

		err := r.Run(strings.Split(cmd, " "), RunOptions{
			Stdout: &b,
			Stderr: &b,
			SetOpts: func(opts map[string]cty.Value, pendingOptions []app.PendingInput) error {
				var elems []slack.DialogElement

				for _, o := range pendingOptions {
					k := o.Name

					var desc string

					if o.Description != nil {
						desc = *o.Description
					}

					var elem slack.DialogElement

					switch o.Type {
					case cty.String, cty.Bool, cty.Number:
						elem = slack.DialogInput{
							Label:       k,
							Placeholder: desc,
							Type:        slack.InputTypeText,
							Name:        k,
							//Optional: true,
						}
					default:
						elem = slack.DialogInput{
							Label:       k,
							Placeholder: desc,
							Type:        slack.InputTypeTextArea,
							Name:        k,
							//Optional: true,
						}
					}

					elems = append(elems, elem)
				}

				callbackID := message.UserID + "_" + message.TriggerID
				dialog := slack.Dialog{
					Title:          cmd,
					SubmitLabel:    "Run",
					CallbackID:     callbackID,
					Elements:       elems,
					NotifyOnCancel: true,
				}

				done := make(chan error, 1)

				bot.RegisterInteractionCallbackHandler(callbackID, func(callback slack.InteractionCallback) (interface{}, error) {
					if callback.Type == slack.InteractionTypeDialogCancellation {
						done <- nil

						return nil, nil
					}

					if callback.Type != slack.InteractionTypeDialogSubmission {
						return nil, fmt.Errorf("unexpected type of interaction callback: want %s, got %s", slack.InteractionTypeDialogSubmission, callback.Type)
					}

					_, transformers, err := app.MakeQuestions(pendingOptions)
					if err != nil {
						return nil, err
					}

					vals := make(map[string]interface{})

					errs := map[string]error{}

					for _, o := range pendingOptions {
						k := o.Name
						v := callback.Submission[k]

						if v == "" {
							errs[k] = fmt.Errorf("%s is required", k)
							continue
						}

						if o.Type == cty.Bool {
							b, err := strconv.ParseBool(v)
							if err != nil {
								errs[k] = err
								continue
							}

							vals[k] = b
						} else {
							vals[k] = v
						}
					}

					if len(errs) > 0 {
						type validateError struct {
							Name  string `json:"name"`
							Error string `json:"error"`
						}

						type validateErrorResponse struct {
							Errors []validateError `json:"errors"`
						}

						var validateErrs []validateError

						for k, e := range errs {
							validateErrs = append(validateErrs, validateError{k, e.Error()})
						}

						errResponse := &validateErrorResponse{
							validateErrs,
						}

						return errResponse, nil
					}

					if err := app.SetOptsFromMap(transformers, opts, vals); err != nil {
						return nil, err
					}

					done <- nil

					return nil, nil
				})

				if err := bot.Client.OpenDialogContext(context.TODO(), message.TriggerID, dialog); err != nil {
					log.Print("open dialog failed: ", err)
					return nil
				}

				return <-done
			},
		})

		out := b.String()

		if err != nil {
			return err.Error()
		}

		return out
	})

	return bot.Run()
}
