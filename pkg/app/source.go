package app

import (
	"context"
	"sync"

	"github.com/zclconf/go-cty/cty"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

func (app *App) checkoutSources(_ *EventLogger, jobCtx *JobContext, sources []Source, concurrency int) error {
	type result struct {
		id  string
		dir string
	}

	inputs := make(chan Source)

	results := make(chan result)

	eg, ctx := errgroup.WithContext(context.Background())

	for i := 0; i < concurrency; i++ {
		eg.Go(func() error {
			for src := range inputs {
				dir, err := app.sourceClient.ExtractSource(ctx, src.Kind, src.Namepsace, src.Name)
				if err != nil {
					return xerrors.Errorf("extracting source: %w", err)
				}

				results <- result{id: src.ID, dir: dir}
			}

			return nil
		})
	}

	srcs := map[string]cty.Value{}

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		for src := range results {
			srcs[src.id] = cty.MapVal(map[string]cty.Value{
				"dir": cty.StringVal(src.dir),
			})
		}

		wg.Done()
	}()

	for _, src := range sources {
		inputs <- src
	}

	if err := eg.Wait(); err != nil {
		return xerrors.Errorf("waiting for workers: %w", err)
	}

	wg.Wait()

	if len(srcs) > 0 {
		// Cuz calling cty.MapVal on an empty map panics by its nature
		jobCtx.evalContext.Variables["source"] = cty.MapVal(srcs)
	}

	return nil
}
