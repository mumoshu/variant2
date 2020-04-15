package app

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
)

type eitherJobRun struct {
	static  *StaticRun
	dynamic *DynamicRun
}

type jobRun struct {
	Name string
	Args map[string]interface{}
}

func staticRunToJob(jobCtx *hcl.EvalContext, run *StaticRun) (*jobRun, error) {
	args := map[string]interface{}{}

	for k := range run.Args {
		var v cty.Value
		if diags := gohcl.DecodeExpression(run.Args[k], jobCtx, &v); diags.HasErrors() {
			return nil, diags
		}

		vv, err := ctyToGo(v)
		if err != nil {
			return nil, err
		}

		args[k] = vv
	}

	return &jobRun{
		Name: run.Name,
		Args: args,
	}, nil
}

func dynamicRunToJob(jobCtx *hcl.EvalContext, run *DynamicRun) (*jobRun, error) {
	args, err := exprToGoMap(jobCtx, run.Args)

	if err != nil {
		return nil, err
	}

	return &jobRun{
		Name: run.Job,
		Args: args,
	}, nil
}
