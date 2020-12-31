package app

import (
	"io/ioutil"

	"github.com/hashicorp/hcl/v2"
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
)

type configFragment struct {
	data   []byte
	key    string
	format string
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
