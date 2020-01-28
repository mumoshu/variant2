package app

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
)

func (app *App) newLogCollector(file string, j JobSpec, jobCtx *hcl.EvalContext) LogCollector {
	logCollector := LogCollector{
		FilePath: file,
		CollectFn: func(evt Event) (*string, bool, error) {
			condVars := map[string]cty.Value{}
			for k, v := range jobCtx.Variables {
				condVars[k] = v
			}
			condVars["event"] = evt.toCty()
			condCtx := jobCtx
			condCtx.Variables = condVars

			for _, c := range j.Log.Collects {
				var condVal cty.Value
				if diags := gohcl2.DecodeExpression(c.Condition, condCtx, &condVal); diags.HasErrors() {
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
				for k, v := range jobCtx.Variables {
					formatVars[k] = v
				}
				formatVars["event"] = evt.toCty()
				formatCtx := jobCtx
				formatCtx.Variables = condVars

				var formatVal cty.Value
				if diags := gohcl2.DecodeExpression(c.Format, formatCtx, &formatVal); diags.HasErrors() {
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
			jobCtx.Variables["log"] = logCty

			for _, f := range j.Log.Forwards {
				_, err := app.execRunInternal(nil, jobCtx, f.Run)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}

	return logCollector
}
