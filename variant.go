package variant

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"

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
	Setup          app.Setup
}

type Setup func() (*Main, error)

type InitParams struct {
	Command string
	Setup   app.Setup
}

type Option func(*Main)

func FromPath(path string, opts ...Option) Setup {
	return func() (*Main, error) {
		if path == "" {
			var err error

			path, err = os.Getwd()
			if err != nil {
				return nil, err
			}
		}

		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		var setup app.Setup

		if info.IsDir() {
			setup = app.FromDir(path)
		} else {
			setup = app.FromFile(path)
		}

		m := &Main{
			Setup: setup,
		}

		if m.Command == "" {
			m.Command = filepath.Base(path)
		}

		for _, o := range opts {
			o(m)
		}

		return m, nil
	}
}

func FromSource(cmd, source string) Setup {
	return func() (*Main, error) {
		if cmd == "" {
			return nil, errors.New("command name must be set when loadling from Variant source file")
		}

		return &Main{
			Command: cmd,
			Setup:   app.FromSources(map[string][]byte{cmd: []byte(source)}),
		}, nil
	}
}

func Load(setup Setup) (*Runner, error) {
	initParams, err := setup()
	if err != nil {
		return nil, err
	}

	m := Init(*initParams)

	return m.createRunner(m.Command, m.Setup)
}

func MustLoad(setup Setup) *Runner {
	r, err := Load(setup)
	if err != nil {
		panic(err)
	}

	return r
}

func New() Main {
	return Init(Main{})
}

type Env struct {
	Args   []string
	Getenv func(name string) string
	Getwd  func() (string, error)
}

func GetPathAndArgsFromEnv(env Env) (string, string, []string) {
	osArgs := env.Args

	var cmd string

	var path string

	if len(osArgs) > 1 {
		file := osArgs[1]
		info, err := os.Stat(file)

		if err == nil && info != nil && !info.IsDir() {
			osArgs = osArgs[2:]

			path = file
			cmd = filepath.Base(file)
		} else {
			osArgs = osArgs[1:]
		}
	} else {
		osArgs = []string{}
	}

	if path == "" {
		dirFromEnv := env.Getenv("VARIANT_DIR")
		if dirFromEnv != "" {
			path = dirFromEnv
		} else {
			var err error

			path, err = env.Getwd()
			if err != nil {
				panic(err)
			}
		}
	}

	return cmd, path, osArgs
}

func Init(m Main) Main {
	if m.Stdout == nil {
		m.Stdout = os.Stdout
	}

	if m.Stderr == nil {
		m.Stderr = os.Stderr
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

	return m
}

type Config struct {
	Parameters func([]string) (map[string]interface{}, error)
	Options    func() map[string]func() interface{}
}

func valueOnChange(cli *cobra.Command, name string, v interface{}) func() interface{} {
	return func() interface{} {
		// This avoids setting "" when the flag is actually missing, so that
		// we can differentiate between when (1)an empty string is specified vs (2)no flag is provided.
		if cli.PersistentFlags().Lookup(name).Changed {
			return v
		}

		return nil
	}
}

func createCobraFlagsFromVariantOptions(cli *cobra.Command, opts []app.OptionSpec, interactive bool) (map[string]func() interface{}, error) {
	lazyOptionValues := map[string]func() interface{}{}

	for i := range opts {
		o := opts[i]

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

			lazyOptionValues[o.Name] = valueOnChange(cli, o.Name, &v)
		case cty.Bool:
			var v bool

			if o.Short != nil {
				cli.PersistentFlags().BoolVarP(&v, o.Name, *o.Short, false, desc)
			} else {
				cli.PersistentFlags().BoolVar(&v, o.Name, false, desc)
			}

			lazyOptionValues[o.Name] = valueOnChange(cli, o.Name, &v)
		case cty.Number:
			var v int

			if o.Short != nil {
				cli.PersistentFlags().IntVarP(&v, o.Name, *o.Short, 0, desc)
			} else {
				cli.PersistentFlags().IntVar(&v, o.Name, 0, desc)
			}

			lazyOptionValues[o.Name] = valueOnChange(cli, o.Name, &v)
		case cty.List(cty.String):
			v := []string{}

			if o.Short != nil {
				cli.PersistentFlags().StringSliceVarP(&v, o.Name, *o.Short, []string{}, desc)
			} else {
				cli.PersistentFlags().StringSliceVar(&v, o.Name, []string{}, desc)
			}

			lazyOptionValues[o.Name] = valueOnChange(cli, o.Name, &v)
		case cty.List(cty.Number):
			v := []int{}

			if o.Short != nil {
				cli.PersistentFlags().IntSliceVarP(&v, o.Name, *o.Short, []int{}, desc)
			} else {
				cli.PersistentFlags().IntSliceVar(&v, o.Name, []int{}, desc)
			}

			lazyOptionValues[o.Name] = valueOnChange(cli, o.Name, &v)
		}

		if !app.IsExpressionEmpty(o.Default) || interactive {

		} else if err := cli.MarkPersistentFlagRequired(o.Name); err != nil {
			panic(err)
		}
	}

	return lazyOptionValues, nil
}

func configureCommand(cli *cobra.Command, root app.JobSpec, interactive bool) (*Config, error) {
	lazyOptionValues, err := createCobraFlagsFromVariantOptions(cli, root.Options, interactive)
	if err != nil {
		return nil, err
	}

	opts := func() map[string]func() interface{} {
		m := map[string]func() interface{}{}
		for name, f := range lazyOptionValues {
			m[name] = f
		}

		return m
	}

	var minArgs int

	var maxArgs int

	lazyParamValues := map[string]func(args []string) (interface{}, error){}

	var hasVarArgs bool

	for i := range root.Parameters {
		maxArgs++

		p := root.Parameters[i]
		r := p.Default.Range()

		if r.Start == r.End {
			minArgs++
		}

		ii := i

		ty, err := typeexpr.TypeConstraint(p.Type)

		if err != nil {
			return nil, err
		}

		var f func([]string, int) (interface{}, error)

		switch ty {
		case cty.Bool:
			f = func(args []string, i int) (interface{}, error) {
				return strconv.ParseBool(args[i])
			}
		case cty.String:
			f = func(args []string, i int) (interface{}, error) {
				return args[i], nil
			}
		case cty.Number:
			f = func(args []string, i int) (interface{}, error) {
				return strconv.Atoi(args[i])
			}
		case cty.List(cty.String):
			if i != len(root.Parameters)-1 {
				return nil, fmt.Errorf("list(string) parameter %q must be positioned at last", p.Name)
			}

			f = func(args []string, i int) (interface{}, error) {
				return args[i:], nil
			}

			hasVarArgs = true
		default:
			return nil, fmt.Errorf("invalid parameter %q: type %s is not supported", p.Name, ty.FriendlyName())
		}

		lazyParamValues[p.Name] = func(args []string) (interface{}, error) {
			if len(args) <= ii {
				return nil, nil
			}

			return f(args, ii)
		}
	}

	if hasVarArgs {
		cli.Args = cobra.MinimumNArgs(minArgs)
	} else {
		cli.Args = cobra.RangeArgs(minArgs, maxArgs)
	}

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

func (m *Main) initApp(setup app.Setup) (*app.App, error) {
	ap, err := app.New(setup)
	if err != nil {
		ap.PrintError(err)
		return nil, err
	}

	ap.Stdout = m.Stdout
	ap.Stderr = m.Stderr

	return ap, nil
}

func (m Main) createRunner(cmd string, setup app.Setup) (*Runner, error) {
	ap, err := m.initApp(setup)
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

	m.initRunner(m2)

	return m2
}

func (m Main) initRunner(r *Runner) {
	siTty := isatty.IsTerminal(os.Stdin.Fd())
	soTty := isatty.IsTerminal(os.Stdout.Fd())

	// Enable prompts for missing inputs when stdin and stdout are connected to a tty
	r.Interactive = siTty && soTty

	if r.Interactive {
		r.SetOpts = app.DefaultSetOpts
	}

	r.goJobs = map[string]Job{}
	r.jobRunProviders = map[string]func(State) JobRun{}

	for jobName := range r.ap.JobByName {
		n := jobName

		r.jobRunProviders[n] = func(st State) JobRun {
			return func(ctx context.Context) error {
				if st.Stdout != nil {
					defer func() {
						if err := st.Stdout.Close(); err != nil {
							panic(err)
						}
					}()
				}

				if st.Stderr != nil {
					defer func() {
						if err := st.Stderr.Close(); err != nil {
							panic(err)
						}
					}()
				}

				r, err := r.ap.Run(n, st.Parameters, st.Options)

				if err != nil {
					return err
				}

				if st.Stdout != nil {
					if _, err := st.Stdout.Write([]byte(r.Stdout)); err != nil {
						return err
					}
				}

				if st.Stderr != nil {
					if _, err := st.Stderr.Write([]byte(r.Stderr)); err != nil {
						return err
					}
				}

				return nil
			}
		}
	}
}

type Runner struct {
	ap         *app.App
	runCmdName string

	runCmd     *cobra.Command
	variantCmd *cobra.Command

	goJobs          map[string]Job
	jobRunProviders map[string]func(State) JobRun

	Interactive bool

	SetOpts app.SetOptsFunc

	mut *sync.Mutex
}

func (r *Runner) Cobra() (*cobra.Command, error) {
	ap, rootCmdName := r.ap, r.runCmdName

	if rootCmdName == "" {
		rootCmdName = "run"
	}

	jobs := map[string]app.JobSpec{}
	jobNames := []string{}

	for jobName, j := range ap.JobByName {
		var name string
		if jobName == "" {
			name = rootCmdName
		} else {
			name = fmt.Sprintf("%s %s", rootCmdName, jobName)
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

		for _, p := range job.Parameters {
			cmdName += fmt.Sprintf(" [%s]", strings.ToUpper(p.Name))
		}

		cli := &cobra.Command{
			Use:   cmdName,
			Short: strings.Split(desc, "\n")[0],
			Long:  desc,
		}

		if job.Private != nil {
			cli.Hidden = *job.Private
		}

		cfg, err := configureCommand(cli, job, r.Interactive)

		if err != nil {
			return nil, err
		}

		cfgs[name] = cfg
		cli.RunE = func(cmd *cobra.Command, args []string) error {
			params, opts, err := getMergedParamsAndOpts(cfgs, name, args)
			if err != nil {
				return err
			}

			_, err = ap.Run(job.Name, params, opts, r.SetOpts)
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

	SetOpts app.SetOptsFunc

	DisableLocking bool
}

// Add adds a job to this runner so that it can later by calling `Job`
func (r Runner) Add(job Job) {
	r.goJobs[job.Name] = job

	if job.Name == "" {
		panic(fmt.Errorf("invalid job name %q", job.Name))
	}

	r.jobRunProviders[job.Name] = func(st State) JobRun {
		return func(ctx context.Context) error {
			return job.Run(ctx, st)
		}
	}
}

// Job prepares a job to be run
func (r Runner) Job(job string, opts State) (JobRun, error) {
	f, ok := r.jobRunProviders[job]
	if !ok {
		return nil, fmt.Errorf("job %q not added", job)
	}

	if opts.Options == nil {
		opts.Options = map[string]interface{}{}
	}

	if opts.Parameters == nil {
		opts.Parameters = map[string]interface{}{}
	}

	jr := f(opts)

	return jr, nil
}

func (r *Runner) Run(arguments []string, opt ...RunOptions) error {
	var opts RunOptions

	if len(opt) > 0 {
		opts = opt[0]
	}

	if !opts.DisableLocking {
		r.mut.Lock()
		defer r.mut.Unlock()
	}

	if opts.SetOpts != nil {
		r.SetOpts = opts.SetOpts

		defer func() {
			r.SetOpts = nil
		}()
	}

	if r.runCmd == nil {
		var err error

		r.runCmd, err = r.Cobra()

		if err != nil {
			r.ap.PrintError(err)

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
		cmdStdout := cmd.OutOrStdout()
		cmdStderr := cmd.OutOrStderr()

		appStdout := r.ap.Stdout
		appStderr := r.ap.Stderr

		cmd.SetArgs(arguments)

		if opts.Stdout != nil {
			cmd.SetOut(opts.Stdout)

			r.ap.Stdout = opts.Stdout
		}

		if opts.Stderr != nil {
			cmd.SetErr(opts.Stderr)

			r.ap.Stderr = opts.Stderr
		}

		err = cmd.Execute()

		cmd.SetOut(cmdStdout)
		cmd.SetErr(cmdStderr)

		r.ap.Stdout = appStdout
		r.ap.Stderr = appStderr
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
	{
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
		exportCmd.AddCommand(newExportGo(r))
		exportCmd.AddCommand(newExportBinary(r))
	}

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

	startCmd := &cobra.Command{
		Use:   "start NAME",
		Short: "Start the named integration to turn the Variant command to whatever",
	}
	{
		var botName string

		startSlackbotCmd := &cobra.Command{
			Use:   "slackbot",
			Short: "Start the slackbot that responds to slash commands by running corresopnding Variant commands",
			RunE: func(c *cobra.Command, args []string) error {
				err := r.StartSlackbot(botName)
				if err != nil {
					c.SilenceUsage = true
				}
				return err
			},
		}

		startSlackbotCmd.Flags().StringVarP(&botName, "name", "n", "", "Name of the slash command without /. For example, \"--name foo\" results in the bot responding to \"/foo <CMD> <ARGS>\"")

		if err := startSlackbotCmd.MarkFlagRequired("name"); err != nil {
			panic(err)
		}

		startCmd.AddCommand(startSlackbotCmd)
	}

	rootCmd.AddCommand(r.runCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(startCmd)

	return rootCmd
}
