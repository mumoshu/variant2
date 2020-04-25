package app

import (
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
)

type eitherJobRun struct {
	static  *StaticRun
	dynamic *DynamicRun
}

type jobRun struct {
	Name    string
	Args    map[string]interface{}
	Skipped bool
}

func staticRunToJob(jobCtx *JobContext, run *StaticRun) (*jobRun, error) {
	localArgs, err := exprMapToGoMap(jobCtx.evalContext, run.Args)
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

	return &jobRun{
		Name: run.Name,
		Args: args,
	}, nil
}

func dynamicRunToJob(jobCtx *JobContext, run *DynamicRun) (*jobRun, error) {
	localArgs, err := exprToGoMap(jobCtx.evalContext, run.Args)
	if err != nil {
		return nil, err
	}

	if !IsExpressionEmpty(run.Condition) {
		var condition bool

		if diags := gohcl2.DecodeExpression(run.Condition, jobCtx.evalContext, &condition); diags.HasErrors() {
			return nil, diags
		}

		if !condition {
			return &jobRun{
				Name:    run.Job,
				Skipped: true,
			}, nil
		}
	}

	args := map[string]interface{}{}

	for k, v := range jobCtx.globalArgs {
		args[k] = v
	}

	for k, v := range localArgs {
		args[k] = v
	}

	return &jobRun{
		Name: run.Job,
		Args: args,
	}, nil
}
