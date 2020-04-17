package app

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

func exprMapToGoMap(ctx *hcl.EvalContext, m map[string]hcl.Expression) (map[string]interface{}, error) {
	args := map[string]interface{}{}

	for k := range m {
		var v cty.Value
		if diags := gohcl.DecodeExpression(m[k], ctx, &v); diags.HasErrors() {
			return nil, diags
		}

		vv, err := ctyToGo(v)
		if err != nil {
			return nil, err
		}

		args[k] = vv
	}

	return args, nil
}

func exprToGoMap(ctx *hcl.EvalContext, expr hcl.Expression) (map[string]interface{}, error) {
	args := map[string]interface{}{}

	// We need to explicitly specify that the type of values is DynamicPseudoType.
	//
	// Otherwise, for e.g. map[string]cty.Value{], DecodeExpression computes the lowest common type for all the values.
	// That is, {"foo":true,"bar":"BAR"} would produce cty.Map(cty.String) = map[string]string,
	// rather than cty.Map(DynamicPseudoType) = map[string]interface{}.
	m := cty.MapValEmpty(cty.DynamicPseudoType)

	if err := gohcl.DecodeExpression(expr, ctx, &m); err != nil {
		return nil, err
	}

	ctyArgs := m.AsValueMap()

	for k, v := range ctyArgs {
		var err error

		args[k], err = ctyToGo(v)

		if err != nil {
			return nil, err
		}
	}

	return args, nil
}

func ctyToGo(v cty.Value) (interface{}, error) {
	var vv interface{}

	switch tpe := v.Type(); tpe {
	case cty.String:
		var vvv string

		if err := gocty.FromCtyValue(v, &vvv); err != nil {
			return nil, err
		}

		vv = vvv
	case cty.Number:
		var vvv int

		if err := gocty.FromCtyValue(v, &vvv); err != nil {
			return nil, err
		}

		vv = vvv
	case cty.Bool:
		var vvv bool

		if err := gocty.FromCtyValue(v, &vvv); err != nil {
			return nil, err
		}

		vv = vvv
	case cty.List(cty.String):
		var vvv []string

		if err := gocty.FromCtyValue(v, &vvv); err != nil {
			return nil, err
		}

		vv = vvv
	case cty.List(cty.Number):
		var vvv []int

		if err := gocty.FromCtyValue(v, &vvv); err != nil {
			return nil, err
		}

		vv = vvv
	case cty.Map(cty.String):
		m := map[string]string{}

		if err := gocty.FromCtyValue(v, &v); err != nil {
			return nil, err
		}

		vv = m
	case cty.Map(cty.DynamicPseudoType):
		m := map[string]interface{}{}

		for k, v := range v.AsValueMap() {
			v, err := ctyToGo(v)
			if err != nil {
				return nil, err
			}
			m[k] = v
		}

		vv = m
	default:
		if tpe.IsTupleType() {
			elemTypes := tpe.TupleElementTypes()

			if len(elemTypes) == 0 {
				vv = []interface{}{}
			} else {
				var lastElemType *cty.Type

				var typeVaries bool

				for i := range elemTypes {
					t := &elemTypes[i]

					if lastElemType == nil {
						lastElemType = t
					} else if !lastElemType.Equals(*t) {
						//return nil, fmt.Errorf("handler for tuple with varying element types is not implemented yet: %v", v)
						typeVaries = true
						break
					}
				}

				if typeVaries {
					var elems []interface{}

					iter := v.ElementIterator()

					for iter.Next() {
						_, elemValue := iter.Element()
						elemGo, err := ctyToGo(elemValue)
						if err != nil {
							return nil, err
						}

						elems = append(elems, elemGo)
					}

					vv = elems
				} else {
					switch *lastElemType {
					case cty.String:
						var vvv []string

						for i := range elemTypes {
							var s string

							if err := gocty.FromCtyValue(v.Index(cty.NumberIntVal(int64(i))), &s); err != nil {
								return nil, err
							}

							vvv = append(vvv, s)
						}

						vv = vvv
					case cty.Number:
						var vvv []int

						for i := range elemTypes {
							var s int

							if err := gocty.FromCtyValue(v.Index(cty.NumberIntVal(int64(i))), &s); err != nil {
								return nil, err
							}

							vvv = append(vvv, s)
						}

						vv = vvv
					default:
						return nil, fmt.Errorf("handler for tuple with element type of %s is not implemented yet: %v", *lastElemType, v)
					}
				}
			}
		} else if tpe.IsObjectType() {
			m := map[string]interface{}{}

			for name := range tpe.AttributeTypes() {
				attr := v.GetAttr(name)

				v, err := ctyToGo(attr)
				if err != nil {
					return nil, fmt.Errorf("unable to decoode attribute %q of object: %w", name, err)
				}
				m[name] = v
			}

			vv = m
		} else {
			return nil, fmt.Errorf("handler for type %s not implemented yet", v.Type().FriendlyName())
		}
	}

	return vv, nil
}

