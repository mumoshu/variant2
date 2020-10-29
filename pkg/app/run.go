package app

import (
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

func (app *App) runJobInBody(l *EventLogger, jobCtx *JobContext, body hcl.Body, streamOutput bool) (*Result, bool, error) {
	var runs []eitherJobRun

	var lazyStaticRun LazyStaticRun

	sErr := gohcl.DecodeBody(body, jobCtx.evalContext, &lazyStaticRun)

	//nolint:nestif
	if sErr.HasErrors() {
		var lazyDynamicRun LazyDynamicRun

		dErr := gohcl.DecodeBody(body, jobCtx.evalContext, &lazyDynamicRun)

		if dErr != nil {
			sErrMsg := sErr.Error()
			if !strings.Contains(sErrMsg, "Missing run block") && !strings.Contains(sErrMsg, "Missing name for run") {
				return nil, false, sErr
			}

			dErrMsg := dErr.Error()
			if !strings.Contains(dErrMsg, "Missing run block") {
				return nil, false, dErr
			}
		} else {
			for i := range lazyDynamicRun.Run {
				r := lazyDynamicRun.Run[i]

				either := eitherJobRun{}

				either.dynamic = &r

				runs = append(runs, either)
			}
		}
	} else {
		for i := range lazyStaticRun.Run {
			r := lazyStaticRun.Run[i]

			either := eitherJobRun{}

			either.static = &r

			runs = append(runs, either)
		}
	}

	if len(runs) == 0 {
		return nil, false, nil
	}

	var results []*Result

	for _, r := range runs {
		res, err := app.runJobAndUpdateContext(l, jobCtx, r, new(sync.Mutex), streamOutput)
		if err != nil {
			return res, true, err
		}

		if res == nil {
			return res, true, nil
		}

		if !res.Skipped {
			results = append(results, res)
		}
	}

	if len(results) == 0 {
		return nil, true, nil
	}

	return aggregateResults(results), true, nil
}

func aggregateResults(results []*Result) *Result {
	aggregated := *results[len(results)-1]

	aggregated.Stdout = ""
	aggregated.Stderr = ""

	for _, r := range results {
		if r.Stdout != "" {
			aggregated.Stdout += r.Stdout
		}

		if r.Stderr != "" {
			aggregated.Stderr += r.Stderr
		}
	}

	return &aggregated
}
