package app

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"
)

func goToCty(goV interface{}) (cty.Value, error) {
	switch typed := goV.(type) {
	case map[string]interface{}:
		m := map[string]cty.Value{}

		for k, v := range typed {
			var err error

			m[k], err = goToCty(v)
			if err != nil {
				return cty.DynamicVal, err
			}
		}

		// cty.MapVal doesn't support empty maps. It panics when encountered an empty map, so...
		if len(m) == 0 {
			return cty.MapValEmpty(cty.DynamicPseudoType), nil
		}

		return cty.MapVal(m), nil
	case map[string]string:
		return strToStrMapToCty(typed)
	case string:
		return cty.StringVal(typed), nil
	case *string:
		if typed == nil {
			return cty.NullVal(cty.String), nil
		}

		return goToCty(*typed)
	case bool:
		return cty.BoolVal(typed), nil
	case *bool:
		if typed == nil {
			return cty.NullVal(cty.Bool), nil
		}

		return goToCty(*typed)
	case int:
		return cty.NumberIntVal(int64(typed)), nil
	case *int:
		if typed == nil {
			return cty.NullVal(cty.Number), nil
		}

		return goToCty(*typed)
	case []string:
		var vs []cty.Value

		for _, s := range typed {
			vs = append(vs, cty.StringVal(s))
		}

		return cty.ListVal(vs), nil
	case *[]string:
		if typed == nil {
			return cty.ListValEmpty(cty.String), nil
		}

		return goToCty(*typed)
	case []int:
		var vs []cty.Value

		for _, i := range typed {
			vs = append(vs, cty.NumberIntVal(int64(i)))
		}

		return cty.ListVal(vs), nil
	case *[]int:
		if typed == nil {
			return cty.ListValEmpty(cty.Number), nil
		}

		return goToCty(*typed)
	case []interface{}:
		if len(typed) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType), nil
		}

		var vs []cty.Value

		for _, v := range typed {
			vv, err := goToCty(v)
			if err != nil {
				return cty.DynamicVal, err
			}

			vs = append(vs, vv)
		}

		return cty.ListVal(vs), nil
	default:
		return cty.DynamicVal, fmt.Errorf("unsupported type of value %v(%T)", typed, typed)
	}
}

func strToStrMapToCty(typed map[string]string) (cty.Value, error) {
	m := map[string]cty.Value{}

	for k, v := range typed {
		var err error

		m[k], err = goToCty(v)
		if err != nil {
			return cty.DynamicVal, err
		}
	}

	// cty.MapVal doesn't support empty maps. It panics when encountered an empty map, so...
	if len(m) == 0 {
		return cty.MapValEmpty(cty.String), nil
	}

	return cty.MapVal(m), nil
}
