package app

type eitherJobRun struct {
	static  *StaticRun
	dynamic *DynamicRun
}

type jobRun struct {
	Name string
	Args map[string]interface{}
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
