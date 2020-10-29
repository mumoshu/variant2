package app

import "github.com/hashicorp/hcl/v2"

func buildArgsFromExpr(jobCtx *JobContext, expr hcl.Expression) (map[string]interface{}, error) {
	localArgs, err := exprToGoMap(jobCtx.evalContext, expr)
	if err != nil {
		return nil, err
	}

	args := map[string]interface{}{}

	for k, v := range jobCtx.globalArgs {
		args[k] = v
	}

	for k, v := range localArgs {
		args[k] = v
	}

	return args, nil
}
