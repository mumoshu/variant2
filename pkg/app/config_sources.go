package app

import (
	"errors"
	"fmt"
	"io/ioutil"
	"reflect"

	"github.com/hashicorp/hcl/v2"
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
	"golang.org/x/xerrors"
)

type configFragment struct {
	data   []byte
	key    string
	format string
}

func loadConfigSourceContent(sourceSpec ConfigSource) (*hcl.BodyContent, error) {
	body := sourceSpec.Body

	var val reflect.Value

	switch sourceSpec.Type {
	case "file":
		rv := reflect.ValueOf(&SourceFile{})
		if rv.Kind() != reflect.Ptr {
			panic(fmt.Sprintf("target value must be a pointer, not %s", rv.Type().String()))
		}

		val = rv.Elem()
	case "job":
		rv := reflect.ValueOf(&SourceJob{})
		if rv.Kind() != reflect.Ptr {
			panic(fmt.Sprintf("target value must be a pointer, not %s", rv.Type().String()))
		}

		val = rv.Elem()
	default:
		return nil, fmt.Errorf("config source %q is not implemented. It must be either \"file\" or \"job\", so that it looks like `source file {` or `source file {`", sourceSpec.Type)
	}

	schema, partial := gohcl2.ImpliedBodySchema(val.Interface())

	var content *hcl.BodyContent
	var _ hcl.Body
	var diags hcl.Diagnostics
	if partial {
		content, _, diags = body.PartialContent(schema)
	} else {
		content, diags = body.Content(schema)
	}
	if content == nil {
		return nil, diags
	}

	return content, nil
}

func (app *App) loadConfigSource(jobCtx *JobContext, confCtx *hcl.EvalContext, sourceSpec ConfigSource) ([]configFragment, error) {
	var err error

	var fragments []configFragment

	switch sourceSpec.Type {
	case "file":
		fragments, err = loadFileConfigSource(confCtx, sourceSpec)
		if err != nil {
			return nil, err
		}
	case "job":
		fragments, err = app.loadJobConfigSource(jobCtx, confCtx, sourceSpec)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("config source %q is not implemented. It must be either \"file\" or \"job\", so that it looks like `source file {` or `source file {`", sourceSpec.Type)
	}

	return fragments, nil
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

	if len(paths) == 0 {
		return nil, errors.New("either path or paths must be specified")
	}

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
