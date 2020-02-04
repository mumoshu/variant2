package variant

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/mumoshu/variant2/pkg/app"
	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
)

var Version string

type Main struct {
	// Command is the name of the executable used for this process.
	// E.g. `go build -o myapp ./` and `./myapp cmd --flag1` results in Command being "myapp".
	Command string
	Source  []byte
	// Path can be a path to the directory or the file containing the definition for the Variant command being run
	Path           string
	Stdout, Stderr io.Writer
	Args           []string
	Getenv         func(string) string
	Getwd          func() (string, error)
}

func Load(path string) (*Runner, error) {
	m := Init(Main{Command: filepath.Base(path), Path: path})

	return m.Runner()
}

func Eval(cmd, source string) (*Runner, error) {
	m := Init(Main{Command: cmd, Source: []byte(source)})

	return m.Runner()
}

func MustEval(cmd, source string) *Runner {
	r, err := Eval(cmd, source)

	if err != nil {
		panic(err)
	}

	return r
}

func New() Main {
	return Init(Main{})
}

func Init(m Main) Main {
	if m.Stdout == nil {
		m.Stdout = os.Stdout
	}

	if m.Stderr == nil {
		m.Stderr = os.Stderr
	}

	if m.Args == nil || len(m.Args) == 0 {
		m.Args = os.Args
	}

	if m.Path == "" && len(m.Args) > 1 {
		file := m.Args[1]
		info, err := os.Stat(file)

		if err == nil && info != nil && !info.IsDir() {
			cmdName := filepath.Base(file)
			args := []string{cmdName}

			m.Command = cmdName

			if len(m.Args) > 2 {
				args = append(args, m.Args[2:]...)
			}

			m.Args = args

			m.Path = file
		}
	}

	if m.Getenv == nil {
		m.Getenv = os.Getenv
	}

	if m.Getwd == nil {
		m.Getwd = os.Getwd
	}

	cmdNameFromEnv := m.Getenv("VARIANT_NAME")
	if cmdNameFromEnv != "" {
		m.Command = cmdNameFromEnv
	}

	dirFromEnv := m.Getenv("VARIANT_DIR")
	if dirFromEnv != "" {
		m.Path = dirFromEnv
	}

	return m
}

type Config struct {
	Parameters func([]string) (map[string]interface{}, error)
	Options    func() map[string]func() interface{}
}

func valueOnChange(cli *cobra.Command, name string, v interface{}, def interface{}) func() interface{} {
	return func() interface{} {
		// This avoids setting "" when the flag is actually missing, so that
		// we can differentiate between when (1)an empty string is specified vs (2)no flag is provided.
		if cli.PersistentFlags().Lookup(name).Changed {
			return v
		}

		return def
	}
}

func configureCommand(cli *cobra.Command, root app.JobSpec) (*Config, error) {
	lazyOptionValues := map[string]func() interface{}{}

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

			lazyOptionValues[o.Name] = valueOnChange(cli, o.Name, &v, nil)
		case cty.Bool:
			var v bool

			if o.Short != nil {
				cli.PersistentFlags().BoolVarP(&v, o.Name, *o.Short, false, desc)
			} else {
				cli.PersistentFlags().BoolVar(&v, o.Name, false, desc)
			}

			lazyOptionValues[o.Name] = valueOnChange(cli, o.Name, &v, &v)
		case cty.Number:
			var v int

			if o.Short != nil {
				cli.PersistentFlags().IntVarP(&v, o.Name, *o.Short, 0, desc)
			} else {
				cli.PersistentFlags().IntVar(&v, o.Name, 0, desc)
			}

			lazyOptionValues[o.Name] = valueOnChange(cli, o.Name, &v, &v)
		case cty.List(cty.String):
			v := []string{}

			if o.Short != nil {
				cli.PersistentFlags().StringSliceVarP(&v, o.Name, *o.Short, []string{}, desc)
			} else {
				cli.PersistentFlags().StringSliceVar(&v, o.Name, []string{}, desc)
			}

			lazyOptionValues[o.Name] = valueOnChange(cli, o.Name, &v, &v)
		case cty.List(cty.Number):
			v := []int{}

			if o.Short != nil {
				cli.PersistentFlags().IntSliceVarP(&v, o.Name, *o.Short, []int{}, desc)
			} else {
				cli.PersistentFlags().IntSliceVar(&v, o.Name, []int{}, desc)
			}

			lazyOptionValues[o.Name] = valueOnChange(cli, o.Name, &v, &v)
		}

		if o.Default.Range().Start != o.Default.Range().End {

		} else if err := cli.MarkPersistentFlagRequired(o.Name); err != nil {
			panic(err)
		}
	}

	var minArgs int

	var maxArgs int

	lazyParamValues := map[string]func(args []string) (interface{}, error){}

	for i := range root.Parameters {
		maxArgs++

		p := root.Parameters[i]
		r := p.Default.Range()

		if r.Start == r.End {
			minArgs++
		}

		ii := i
		lazyParamValues[p.Name] = func(args []string) (interface{}, error) {
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

		for name, f := range lazyParamValues {
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
		for name, f := range lazyOptionValues {
			m[name] = f
		}

		return m
	}

	return &Config{Parameters: params, Options: opts}, nil
}

func getMergedParamsAndOpts(
	cfgs map[string]*Config, cmdName string, args []string) (map[string]interface{}, map[string]interface{}, error) {
	names := strings.Split(cmdName, " ")
	optGetters := map[string]func() interface{}{}

	for i := range names {
		curName := strings.Join(names[:i+1], " ")
		if curCfg, ok := cfgs[curName]; ok {
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

func (m Main) Run() error {
	r, err := m.Runner()
	if err != nil {
		return err
	}

	return r.Run(m.Args[1:], RunOptions{})
}

func (m Main) Runner() (*Runner, error) {
	var m2 *Runner

	if m.Source != nil {
		var err error

		if m.Command == "" {
			return nil, errors.New("Main.Command must be set when loadling from Variant source file")
		}

		m2, err = m.runnerFromSource(m.Command, m.Source)

		if err != nil {
			return nil, err
		}
	}

	if m2 == nil {
		path := m.Path

		if path == "" {
			var err error

			path, err = m.Getwd()

			if err != nil {
				return nil, err
			}
		}

		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		cmd := m.Command

		if info.IsDir() {
			m2, err = m.runnerFromDir(cmd, path)
		} else {
			m2, err = m.runnerFromFile(cmd, path)
		}

		if err != nil {
			return nil, err
		}
	}

	return m2, nil
}

func (m Main) runnerFromDir(cmd string, dir string) (*Runner, error) {
	ap, err := m.initAppFromDir(dir)
	if err != nil {
		return nil, err
	}

	return m.newRunner(ap, cmd), nil
}

func (m Main) runnerFromFile(cmd string, file string) (*Runner, error) {
	ap, err := m.initAppFromFile(file)
	if err != nil {
		return nil, err
	}

	return m.newRunner(ap, cmd), nil
}

func (m Main) runnerFromSource(cmd string, code []byte) (*Runner, error) {
	ap, err := m.initAppFromSource(cmd, code)
	if err != nil {
		return nil, err
	}

	return m.newRunner(ap, cmd), nil
}

func (m Main) newRunner(ap *app.App, cmdName string) *Runner {
	m2 := &Runner{
		mut:        &sync.Mutex{},
		ap:         ap,
		runCmdName: cmdName,
	}

	return m2
}

type Runner struct {
	ap         *app.App
	runCmdName string

	runCmd     *cobra.Command
	variantCmd *cobra.Command

	mut *sync.Mutex
}

func (r *Runner) Cobra() (*cobra.Command, error) {
	ap, rootCmdName := r.ap, r.runCmdName

	if rootCmdName == "" {
		rootCmdName = "run"
	}

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

type RunOptions struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Add adds a job to this runner so that it can later by calling `Job`
func (r Runner) Add(job Job) {

}

// Job prepares a job to be run
func (r Runner) Job(job string, opts JobOptions) (*JobRun, error) {
	return nil, nil
}

func (r Runner) Run(arguments []string, opt ...RunOptions) error {
	r.mut.Lock()
	defer r.mut.Unlock()

	var opts RunOptions

	if len(opt) > 0 {
		opts = opt[0]
	}

	if r.runCmd == nil {
		var err error

		r.runCmd, err = r.Cobra()

		if err != nil {
			return err
		}
	}

	var cmd *cobra.Command

	if r.runCmdName != "" {
		cmd = r.runCmd
	} else {
		if r.variantCmd == nil {
			r.variantCmd = r.createVariantRootCommand()
		}

		cmd = r.variantCmd
	}

	var err error

	{
		stdout := cmd.OutOrStdout()
		stderr := cmd.OutOrStderr()

		cmd.SetArgs(arguments)

		if opts.Stdout != nil {
			cmd.SetOut(opts.Stdout)
		}

		if opts.Stderr != nil {
			cmd.SetErr(opts.Stderr)
		}

		err = cmd.Execute()

		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
	}

	return err
}

type Error struct {
	Message  string
	ExitCode int
}

func (e Error) Error() string {
	return e.Message
}

func (r *Runner) createVariantRootCommand() *cobra.Command {
	const VariantBinName = "variant"

	rootCmd := &cobra.Command{
		Use:     VariantBinName,
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
			_, err := r.ap.RunTests(prefix)
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
			err := r.ap.ExportShim(args[0], args[1])
			if err != nil {
				c.SilenceUsage = true
			}
			return err
		},
	}
	exportCmd.AddCommand(shimCmd)

	generateCmd := &cobra.Command{
		Use:   "generate RESOURCE DIR",
		Short: "Generate RESOURCE for the Variant command defined in DIR",
	}
	{
		generateShimCmd := &cobra.Command{
			Use:   "shim DIR",
			Short: "Generate a shim for the Variant command defined in DIR",
			Args:  cobra.ExactArgs(1),
			RunE: func(c *cobra.Command, args []string) error {
				err := app.GenerateShim(VariantBinName, args[0])
				if err != nil {
					c.SilenceUsage = true
				}
				return err
			},
		}
		generateCmd.AddCommand(generateShimCmd)
	}

	rootCmd.AddCommand(r.runCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(generateCmd)

	return rootCmd
}
