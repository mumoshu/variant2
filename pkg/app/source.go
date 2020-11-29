package app

import (
	"context"
	"sync"

	"github.com/zclconf/go-cty/cty"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

func (app *App) checkoutSources(_ *EventLogger, jobCtx *JobContext, sources []Source, concurrency int) error {
	if len(sources) == 0 {
		return nil
	}

	type result struct {
		id  string
		dir string
	}

	sourceCh := make(chan Source)

	resultCh := make(chan result)

	sourceWorkers, ctx := errgroup.WithContext(context.Background())

	for i := 0; i < concurrency; i++ {
		sourceWorkers.Go(func() error {
			for src := range sourceCh {
				dir, err := app.sourceClient.ExtractSource(ctx, src.Kind, src.Namepsace, src.Name)
				if err != nil {
					return xerrors.Errorf("extracting source: %w", err)
				}

				resultCh <- result{id: src.ID, dir: dir}
			}

			return nil
		})
	}

	results := map[string]cty.Value{}

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		for src := range resultCh {
			results[src.id] = cty.MapVal(map[string]cty.Value{
				"dir": cty.StringVal(src.dir),
			})
		}

		wg.Done()
	}()

	for _, src := range sources {
		sourceCh <- src
	}

	close(sourceCh)

	if err := sourceWorkers.Wait(); err != nil {
		return xerrors.Errorf("waiting for workers: %w", err)
	}

	close(resultCh)

	wg.Wait()

	if len(results) > 0 {
		// Cuz calling cty.MapVal on an empty map panics by its nature
		jobCtx.evalContext.Variables["source"] = cty.MapVal(results)
	}

	return nil
}
