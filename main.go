package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/spf13/cobra"
	"github.com/mumoshu/hcl2test/pkg/app"
	"github.com/zclconf/go-cty/cty"
)

func main() {
	err := Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v", err)
		os.Exit(1)
	}
}

func configureCommand(cli *cobra.Command, root app.JobSpec) (func([]string) map[string]interface{}, func() map[string]interface{}, error) {
	options := map[string]interface{}{}
	optionFeeds := map[string]func() interface{}{}
	for _, o := range root.Options {
		var tpe cty.Type
		tpe, diags := typeexpr.TypeConstraint(o.Type)
		if diags != nil {
			return nil, nil, diags
		}
		switch tpe {
		case cty.String:
			var v string
			cli.Flags().StringVar(&v, o.Name, "", o.Description)
			options[o.Name] = &v
			optionFeeds[o.Name] = func() interface{} {
				return v
			}
		case cty.Bool:
			var v bool
			cli.Flags().BoolVar(&v, o.Name, false, o.Description)
			options[o.Name] = &v
			optionFeeds[o.Name] = func() interface{} {
				return v
			}
		}
		if o.Default.Range().Start != o.Default.Range().End {

		} else {
			cli.MarkFlagRequired(o.Name)
		}
	}
	var minArgs int
	var maxArgs int
	paramFeeds := map[string]func(args []string) (interface{}, error){}
	for i, p := range root.Parameters {
		maxArgs += 1
		if p.Default.Range().Start != p.Default.Range().End {
			minArgs += 1
		} else {

		}
		ii := i
		paramFeeds[p.Name] = func(args []string) (interface{}, error) {
			v := args[ii]
			ty, err := typeexpr.TypeConstraint(p.Type)
			if err != nil {
				return nil, err
			}
			switch ty {
			case cty.Bool:
				return strconv.ParseBool(v)
			case cty.String:
				return v, nil
			case cty.Number:
				return strconv.Atoi(v)
			}
			return nil, fmt.Errorf("unexpected type of arg at %d: value=%v, type=%T", ii, v, v)
		}
	}
	cli.Args = cobra.RangeArgs(minArgs, maxArgs)
	params := func(args []string) map[string]interface{} {
		m := map[string]interface{}{}
		for name, f := range paramFeeds {
			v, err := f(args)
			if err != nil {
				panic(err)
			}
			m[name] = v
		}
		return m
	}
	opts := func() map[string]interface{} {
		m := map[string]interface{}{}
		for name, f := range optionFeeds {
			m[name] = f()
		}
		return m
	}
	return params, opts, nil
}

func Run(osArgs []string) error {
	dir := os.Getenv("VARIANT_DIR")

	if dir == "" {
		panic("VARIANT_DIR must be set")
	}

	ap, err := app.New(dir)

	if err != nil {
		ap.ExitWithError(err)
	}

	var rootName string
	if rootName == "" {
		rootName = os.Getenv("VARIANT_NAME")
	}

	if rootName == "" {
		panic("rootName must be set")
	}

	var jobs map[string]app.JobSpec
	jobs[rootName] = ap.Config.JobSpec
	for _, j := range ap.Config.Jobs {
		name := fmt.Sprintf("%s %s", rootName, j.Name)
		jobs[name] = j
	}

	commands := map[string]*cobra.Command{}

	for name, job := range jobs {
		cli := &cobra.Command{
			Use:   name,
			Short: strings.Split(job.Description, "\n")[0],
			Long:  job.Description,
		}
		getParams, getOpts, err := configureCommand(cli, job)
		if err != nil {
			return err
		}
		cli.RunE = func(cmd *cobra.Command, osargs []string) error {
			params := getParams(osArgs)
			opts := getOpts()
			runs, err := ap.Run(job.Name, params, opts)
			if len(runs.Steps) == 0 {
				return fmt.Errorf("Nothing to run. Printing usage.")
			}
			cmd.SilenceUsage = true
			return err
		}
		commands[name] = cli
	}

	return commands[rootName].Execute()
	//
	//if err := ap.Run(cmd, args, opts); err != nil {
	//	ap.ExitWithError(err)
	//}
}
