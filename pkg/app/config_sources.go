package app

import (
	"io/ioutil"

	"github.com/hashicorp/hcl/v2"
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
	"golang.org/x/xerrors"
)

type configFragment struct {
	data   []byte
	key    string
	format string
}

func (app *App) loadJobConfigSource(jobCtx *JobContext, confCtx *hcl.EvalContext, sourceSpec ConfigSource) ([]configFragment, error) {
	var source SourceJob
	if err := gohcl2.DecodeBody(sourceSpec.Body, confCtx, &source); err != nil {
		return nil, xerrors.Errorf("decoding job body: %w", err)
	}

	args, err := buildArgsFromExpr(jobCtx.WithEvalContext(confCtx).Ptr(), source.Args)
	if err != nil {
		return nil, err
	}

	res, err := app.run(jobCtx, nil, source.Name, args, false)
	if err != nil {
		return nil, err
	}

	yamlData := []byte(res.Stdout)

	var (
		format string
		key    string
	)

	if source.Format != nil {
		format = *source.Format
	} else {
		format = FormatYAML
	}

	if source.Key != nil {
		key = *source.Key
	}

	fragments := []configFragment{
		{
			data:   yamlData,
			key:    key,
			format: format,
		},
	}

	return fragments, nil
}

func loadFileConfigSource(confCtx *hcl.EvalContext, sourceSpec ConfigSource) ([]configFragment, error) {
	var source SourceFile
	if err := gohcl2.DecodeBody(sourceSpec.Body, confCtx, &source); err != nil {
		return nil, err
	}

	format := FormatYAML

	var key string

	if source.Key != nil {
		key = *source.Key
	}

	var paths []string

	if p := source.Path; p != nil && *p != "" {
		paths = append(paths, *p)
	}

	paths = append(paths, source.Paths...)

	var fragments []configFragment

	for _, path := range paths {
		yamlData, err := ioutil.ReadFile(path)
		if err != nil {
			if source.Default == nil {
				return nil, err
			}

			yamlData = []byte(*source.Default)
		}

		fragments = append(fragments, configFragment{
			data:   yamlData,
			key:    key,
			format: format,
		})
	}

	return fragments, nil
}
