package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/mumoshu/hcl2test/pkg/app"
	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
)

var Version string

type Main struct {
	Command        string
	Source         []byte
	Stdout, Stderr io.Writer
	Args           []string
	Getenv         func(string) string
	Getwd          func() (string, error)
}

func main() {
	m := Main{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Args:   os.Args,
		Getenv: os.Getenv,
		Getwd:  os.Getwd,
	}
	err := m.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v", err)
		os.Exit(1)
	}
}

type Config struct {
	Parameters func([]string) (map[string]interface{}, error)
	Options    func() map[string]func() interface{}
}

func configureCommand(cli *cobra.Command, root app.JobSpec) (*Config, error) {
	options := map[string]interface{}{}
	optionFeeds := map[string]func() interface{}{}
	for i := range root.Options {
		o := root.Options[i]
		var tpe cty.Type
		tpe, diags := typeexpr.TypeConstraint(o.Type)
		if diags != nil {
			return nil, diags
		}
		var desc string
		if o.Description != nil {
			desc = *o.Description
		}
		switch tpe {
		case cty.String:
			var v string
			if o.Short != nil {
				cli.PersistentFlags().StringVarP(&v, o.Name, *o.Short, "", desc)
			} else {
				cli.PersistentFlags().StringVar(&v, o.Name, "", desc)
			}
			options[o.Name] = &v
			optionFeeds[o.Name] = func() interface{} {
				// This avoids setting "" when the flag is actually missing, so that
				// we can differentiate between when (1)an empty string is specified vs (2)no flag is provided.
				if cli.PersistentFlags().Lookup(o.Name).Changed {
					return v
				}
				return nil
			}
		case cty.Bool:
			var v bool
			if o.Short != nil {
				cli.PersistentFlags().BoolVarP(&v, o.Name, *o.Short, false, desc)
			} else {
				cli.PersistentFlags().BoolVar(&v, o.Name, false, desc)
			}
			options[o.Name] = &v
			optionFeeds[o.Name] = func() interface{} {
				// This avoids setting "" when the flag is actually missing, so that
				// we can differentiate between when (1)an empty string is specified vs (2)no flag is provided.
				if cli.PersistentFlags().Lookup(o.Name).Changed {
					return v
				}
				return v
			}
		case cty.Number:
			var v int
			if o.Short != nil {
				cli.PersistentFlags().IntVarP(&v, o.Name, *o.Short, 0, desc)
			} else {
				cli.PersistentFlags().IntVar(&v, o.Name, 0, desc)
			}
			options[o.Name] = &v
			optionFeeds[o.Name] = func() interface{} {
				// This avoids setting "" when the flag is actually missing, so that
				// we can differentiate between when (1)an empty string is specified vs (2)no flag is provided.
				if cli.PersistentFlags().Lookup(o.Name).Changed {
					return v
				}
				return v
			}
		case cty.List(cty.String):
			v := []string{}
			if o.Short != nil {
				cli.PersistentFlags().StringSliceVarP(&v, o.Name, *o.Short, []string{}, desc)
			} else {
				cli.PersistentFlags().StringSliceVar(&v, o.Name, []string{}, desc)
			}
			options[o.Name] = &v
			optionFeeds[o.Name] = func() interface{} {
				// This avoids setting "" when the flag is actually missing, so that
				// we can differentiate between when (1)an empty string is specified vs (2)no flag is provided.
				if cli.PersistentFlags().Lookup(o.Name).Changed {
					return v
				}
				return v
			}
		case cty.List(cty.Number):
			v := []int{}
			if o.Short != nil {
				cli.PersistentFlags().IntSliceVarP(&v, o.Name, *o.Short, []int{}, desc)
			} else {
				cli.PersistentFlags().IntSliceVar(&v, o.Name, []int{}, desc)
			}
			options[o.Name] = &v
			optionFeeds[o.Name] = func() interface{} {
				// This avoids setting "" when the flag is actually missing, so that
				// we can differentiate between when (1)an empty string is specified vs (2)no flag is provided.
				if cli.PersistentFlags().Lookup(o.Name).Changed {
					return v
				}
				return v
			}
		}
		if o.Default.Range().Start != o.Default.Range().End {

		} else {
			cli.MarkPersistentFlagRequired(o.Name)
		}
	}
	var minArgs int
	var maxArgs int
	paramFeeds := map[string]func(args []string) (interface{}, error){}
	for i := range root.Parameters {
		maxArgs += 1
		p := root.Parameters[i]
		r := p.Default.Range()
		if r.Start == r.End {
			minArgs += 1
		}
		ii := i
		paramFeeds[p.Name] = func(args []string) (interface{}, error) {
			if len(args) <= ii {
				return nil, nil
			}
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
	params := func(args []string) (map[string]interface{}, error) {
		m := map[string]interface{}{}
		for name, f := range paramFeeds {
			v, err := f(args)
			if err != nil {
				return nil, err
			}
			m[name] = v
		}
		return m, nil
	}
	opts := func() map[string]func() interface{} {
		m := map[string]func() interface{}{}
		for name, f := range optionFeeds {
			m[name] = f
		}
		return m
	}
	return &Config{Parameters: params, Options: opts}, nil
}

func getMergedParamsAndOpts(cfgs map[string]*Config, cmdName string, args []string) (map[string]interface{}, map[string]interface{}, error) {
	names := strings.Split(cmdName, " ")
	optGetters := map[string]func() interface{}{}
	for i := range names {
		curName := strings.Join(names[:i+1], " ")
		curCfg, ok := cfgs[curName]
		if ok {
			curOpts := curCfg.Options()
			for n := range curOpts {
				optGetters[n] = curOpts[n]
			}
		}
	}
	cfg := cfgs[cmdName]
	params, err := cfg.Parameters(args)
	if err != nil {
		return nil, nil, err
	}
	opts := map[string]interface{}{}
	for n, get := range optGetters {
		opts[n] = get()
	}
	return params, opts, nil
}

func (m *Main) initAppFromDir(dir string) (*app.App, error) {
	ap, err := app.New(dir)
	if err != nil {
		ap.PrintError(err)
		return nil, err
	}
	ap.Stdout = m.Stdout
	ap.Stderr = m.Stderr

	return ap, nil
}

func (m *Main) initAppFromFile(file string) (*app.App, error) {
	ap, err := app.NewFromFile(file)
	if err != nil {
		ap.PrintError(err)
		return nil, err
	}
	ap.Stdout = m.Stdout
	ap.Stderr = m.Stderr

	return ap, nil
}

func (m *Main) initAppFromSource(cmd string, code []byte) (*app.App, error) {
	ap, err := app.NewFromSources(map[string][]byte{cmd: code})
	if err != nil {
		ap.PrintError(err)
		return nil, err
	}
	ap.Stdout = m.Stdout
	ap.Stderr = m.Stderr

	return ap, nil
}

func (m *Main) initCommand(ap *app.App, rootCmdName string) (*cobra.Command, error) {
	jobs := map[string]app.JobSpec{}
	jobNames := []string{}
	for _, j := range ap.JobByName {
		var name string
		if j.Name == "" {
			name = rootCmdName
		} else {
			name = fmt.Sprintf("%s %s", rootCmdName, j.Name)
		}

		jobs[name] = j

		jobNames = append(jobNames, name)
	}

	sort.Strings(jobNames)

	commands := map[string]*cobra.Command{}
	cfgs := map[string]*Config{}

	for _, n := range jobNames {
		name := n
		job := jobs[name]
		names := strings.Split(name, " ")
		var parent *cobra.Command
		cmdName := names[len(names)-1]
		switch len(names) {
		case 1:
		default:
			names = names[:len(names)-1]
			var ok bool
			parent, ok = commands[strings.Join(names, " ")]
			if !ok {
				for i := range names {
					intName := strings.Join(names[:i+1], " ")
					cur, ok := commands[intName]
					if !ok {
						cur = &cobra.Command{
							Use: names[i],
						}
						parent.AddCommand(cur)
						commands[intName] = cur
					}
					parent = cur
				}
			}
		}
		var desc string
		if job.Description != nil {
			desc = *job.Description
		}
		cli := &cobra.Command{
			Use:   cmdName,
			Short: strings.Split(desc, "\n")[0],
			Long:  desc,
		}
		cfg, err := configureCommand(cli, job)
		if err != nil {
			return nil, err
		}
		cfgs[name] = cfg
		cli.RunE = func(cmd *cobra.Command, args []string) error {
			params, opts, err := getMergedParamsAndOpts(cfgs, name, args)
			if err != nil {
				return err
			}
			_, err = ap.Run(job.Name, params, opts)
			if err != nil && err.Error() != app.NoRunMessage {
				cmd.SilenceUsage = true
			}
			return err
		}
		commands[name] = cli
		if parent != nil {
			parent.AddCommand(cli)
		}
	}

	rootCmd := commands[rootCmdName]

	return rootCmd, nil
}

func (m Main) Run() error {
	cmdNameFromEnv := m.Getenv("VARIANT_NAME")
	if cmdNameFromEnv != "" {
		m.Command = cmdNameFromEnv
	}

	if m.Source != nil {
		return m.runSource(m.Command, m.Source)
	}

	if len(m.Args) > 1 {
		file := m.Args[1]
		info, err := os.Stat(file)
		if err == nil && info != nil && !info.IsDir() {
			cmdName := filepath.Base(file)
			args := []string{cmdName}
			if len(m.Args) > 2 {
				args = append(args, m.Args[2:]...)
			}
			m.Args = args
			m.Command = cmdName
			return m.runFile(cmdName, file)
		}
	}

	dirFromEnv := m.Getenv("VARIANT_DIR")

	dir := dirFromEnv

	if dir == "" {
		var err error
		dir, err = m.Getwd()
		if err != nil {
			return err
		}
	}

	return m.runDir(dir)
}

func (m Main) runDir(dir string) error {
	var cmdName string
	if m.Command != "" {
		cmdName = m.Command
	} else {
		cmdName = "run"
	}

	ap, err := m.initAppFromDir(dir)
	if err != nil {
		return err
	}

	return m.runApp(ap, cmdName)
}

func (m Main) runFile(asCmd, file string) error {
	ap, err := m.initAppFromFile(file)
	if err != nil {
		return err
	}

	return m.runApp(ap, asCmd)
}

func (m Main) runSource(asCmd string, code []byte) error {
	ap, err := m.initAppFromSource(asCmd, code)
	if err != nil {
		return err
	}

	return m.runApp(ap, asCmd)
}

func (m Main) runApp(ap *app.App, cmdName string) error {
	runRootCmd, err := m.initCommand(ap, cmdName)
	if err != nil {
		return err
	}

	if m.Command != "" {
		runRootCmd.SetArgs(m.Args[1:])
		return runRootCmd.Execute()
	}

	rootCmd := &cobra.Command{
		Use:     "variant",
		Version: Version,
	}
	testCmd := &cobra.Command{
		Use:   "test [NAME]",
		Short: "Run test(s)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			var prefix string
			if len(args) > 0 {
				prefix = args[0]
			}
			_, err := ap.RunTests(prefix)
			if err != nil {
				c.SilenceUsage = true
			}
			return err
		},
	}
	exportCmd := &cobra.Command{
		Use:   "export SUBCOMMAND SRC_DIR OUTPUT_PATH",
		Short: "Export the Variant command defined in SRC_DIR to OUTPUT_PATH",
	}
	shimCmd := &cobra.Command{
		Use:   "shim SRC_DIR DST_DIR",
		Short: "Copy and generate shim for the Variant command defined in the SRC",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			err := ap.ExportShim(args[0], args[1])
			if err != nil {
				c.SilenceUsage = true
			}
			return err
		},
	}
	exportCmd.AddCommand(shimCmd)

	rootCmd.AddCommand(runRootCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.SetArgs(m.Args[1:])
	return rootCmd.Execute()
}
