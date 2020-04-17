package app

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

type Arg struct {
	name string

	typeExpr hcl.Expression

	defaultExpr hcl.Expression

	desc *string
}

func setValues(subject string, args map[string]cty.Value, ctx cty.Value, as []Arg, given map[string]interface{}, f SetOptsFunc) error {
	var pendingInputs []PendingInput

	for _, arg := range as {
		v, tpe, err := getValueFor(ctx, arg.name, arg.typeExpr, arg.defaultExpr, given)
		if err != nil {
			return fmt.Errorf("%s %q: %w", subject, arg.name, err)
		}

		if v == nil {
			if f != nil {
				pendingInputs = append(pendingInputs, PendingInput{Name: arg.name, Description: arg.desc, Type: *tpe})
			} else {
				return fmt.Errorf("%s %q: missing value", subject, arg.name)
			}

			continue
		}

		args[arg.name] = *v
	}

	if len(pendingInputs) > 0 {
		if err := f(args, pendingInputs); err != nil {
			return fmt.Errorf("fulfilling missing %s from user input: %w", subject, err)
		}
	}

	return nil
}

func setParameterValues(subject string, ctx cty.Value, specs []Parameter, overrides map[string]interface{}) (map[string]cty.Value, error) {
	values := map[string]cty.Value{}

	{
		var args []Arg

		for _, p := range specs {
			args = append(args, Arg{
				name:        p.Name,
				desc:        p.Description,
				typeExpr:    p.Type,
				defaultExpr: p.Default,
			})
		}

		if err := setValues(subject, values, ctx, args, overrides, nil); err != nil {
			return nil, err
		}
	}

	return values, nil
}


func setOptionValues(subject string, ctx cty.Value, specs []OptionSpec, overrides map[string]interface{}, f SetOptsFunc) (map[string]cty.Value, error) {
	values := map[string]cty.Value{}

	{
		var args []Arg

		for _, p := range specs {
			args = append(args, Arg{
				name:        p.Name,
				desc:        p.Description,
				typeExpr:    p.Type,
				defaultExpr: p.Default,
			})
		}

		if err := setValues(subject, values, ctx, args, overrides, f); err != nil {
			return nil, err
		}
	}

	return values, nil
}

