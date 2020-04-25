package app

import (
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

func (app *App) runJobInBody(l *EventLogger, jobCtx *JobContext, body hcl.Body, streamOutput bool) (*Result, bool, error) {
	either := eitherJobRun{}

	var lazyDynamicRun LazyDynamicRun

	var lazyStaticRun LazyStaticRun

	sErr := gohcl.DecodeBody(body, jobCtx.evalContext, &lazyStaticRun)

	if sErr.HasErrors() {
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
			either.dynamic = &lazyDynamicRun.Run
		}
	} else {
		either.static = &lazyStaticRun.Run
	}

	if either.static != nil || either.dynamic != nil {
		res, err := app.runJobAndUpdateContext(l, jobCtx, either, new(sync.Mutex), streamOutput)

		return res, true, err
	}

	return nil, false, nil
}
