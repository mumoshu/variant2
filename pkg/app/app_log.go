package app

import (
	"fmt"

	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
)

func (app *App) newLogCollector(file string, j JobSpec, jobCtx *JobContext) LogCollector {
	logCollector := LogCollector{
		FilePath: file,
		CollectFn: func(evt Event) (*string, bool, error) {
			condVars := map[string]cty.Value{}
			for k, v := range jobCtx.evalContext.Variables {
				condVars[k] = v
			}
			condVars["event"] = evt.toCty()
			condCtx := *jobCtx.evalContext
			condCtx.Variables = condVars

			for _, c := range j.Log.Collects {
				var condVal cty.Value
				if diags := gohcl2.DecodeExpression(c.Condition, &condCtx, &condVal); diags.HasErrors() {
					return nil, false, diags
				}
				vv, err := ctyToGo(condVal)
				if err != nil {
					return nil, false, err
				}

				b, ok := vv.(bool)
				if !ok {
					return nil, false, fmt.Errorf("unexpected type of condition value: want bool, got %T", vv)
				}

				if !b {
					continue
				}

				formatVars := map[string]cty.Value{}
				for k, v := range jobCtx.evalContext.Variables {
					formatVars[k] = v
				}
				formatVars["event"] = evt.toCty()
				formatCtx := *jobCtx.evalContext
				formatCtx.Variables = condVars

				var formatVal cty.Value
				if diags := gohcl2.DecodeExpression(c.Format, &formatCtx, &formatVal); diags.HasErrors() {
					return nil, false, diags
				}
				formatV, err := ctyToGo(formatVal)
				if err != nil {
					return nil, false, err
				}
				f, ok := formatV.(string)
				if !ok {
					return nil, false, fmt.Errorf("unexpected type of format value: want string, got %T", f)
				}

				return &f, true, nil
			}

			return nil, false, nil
		},
		ForwardFn: func(log Log) error {
			logCty := cty.MapVal(map[string]cty.Value{
				"file": cty.StringVal(log.File),
			})
			evalCtx := *jobCtx.evalContext
			evalCtx.Variables["log"] = logCty

			newJobCtx := *jobCtx
			newJobCtx.evalContext = &evalCtx

			for _, f := range j.Log.Forwards {
				_, err := app.dispatchRunJob(nil, &newJobCtx, eitherJobRun{static: f.Run}, false)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}

	return logCollector
}
