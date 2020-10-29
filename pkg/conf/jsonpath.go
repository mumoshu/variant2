package conf

import (
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"golang.org/x/xerrors"
)

func getValueAtJSONPath(data, path string) (cty.Value, error) {
	result := gjson.Get(data, path)
	if !result.Exists() {
		return cty.NullVal(cty.String), fmt.Errorf("no value found at jsonpath %q: not found", path)
	}

	raw := []byte(result.Raw)

	ty, err := ctyjson.ImpliedType(raw)
	if err != nil {
		return cty.DynamicVal, xerrors.Errorf("determining implied type of %s: %w", string(raw), err)
	}

	return ctyjson.Unmarshal(raw, ty)
}

// JSONPathFunc takes JSON and a query to fetch the value for the query.
var JSONPathFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name: "data",
			Type: cty.String,
		},
		{
			Name: "query",
			Type: cty.String,
		},
	},
	VarParam: &function.Parameter{
		Name: "file",
		Type: cty.String,
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		data := args[0].AsString()
		query := args[1].AsString()

		v, err := getValueAtJSONPath(data, query)

		return v.Type(), err
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		data := args[0].AsString()
		query := args[1].AsString()

		v, err := getValueAtJSONPath(data, query)

		return v, err
	},
})
