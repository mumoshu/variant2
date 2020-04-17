package app

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/kr/text"

	"github.com/zclconf/go-cty/cty/function"

	"github.com/variantdev/mod/pkg/depresolver"

	"github.com/imdario/mergo"
	"github.com/variantdev/mod/pkg/variantmod"
	ctyyaml "github.com/zclconf/go-cty-yaml"
	"gopkg.in/yaml.v3"

	multierror "github.com/hashicorp/go-multierror"
	hcl2 "github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
	hcl2parse "github.com/hashicorp/hcl/v2/hclparse"
	"github.com/mumoshu/variant2/pkg/conf"
	"github.com/pkg/errors"
	"github.com/variantdev/dag/pkg/dag"
	"github.com/variantdev/mod/pkg/shell"
	"github.com/variantdev/vals"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

const (
	NoRunMessage = "nothing to run"

	FormatYAML = "yaml"

	FormatText = "text"
)

type hcl2Loader struct {
	Parser *hcl2parse.Parser
}

type configurable struct {
	Body hcl2.Body
}

func (l hcl2Loader) loadFile(filenames ...string) (*configurable, map[string]*hcl2.File, error) {
	srcs := map[string][]byte{}

	for _, filename := range filenames {
		src, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, nil, err
		}

		srcs[filename] = src
	}

	return l.loadSources(srcs)
}

func (l hcl2Loader) loadSources(srcs map[string][]byte) (*configurable, map[string]*hcl2.File, error) {
	nameToFiles := map[string]*hcl2.File{}

	var files []*hcl2.File

	var diags hcl2.Diagnostics

	for filename, src := range srcs {
		var f *hcl2.File

		var ds hcl2.Diagnostics

		if strings.HasSuffix(filename, ".json") {
			f, ds = l.Parser.ParseJSON(src, filename)
		} else {
			f, ds = l.Parser.ParseHCL(src, filename)
		}

		nameToFiles[filename] = f
		files = append(files, f)
		diags = append(diags, ds...)
	}

	if diags.HasErrors() {
		return nil, nameToFiles, diags
	}

	body := hcl2.MergeFiles(files)

	return &configurable{
		Body: body,
	}, nameToFiles, nil
}

type Config struct {
	Name string `hcl:"name,label"`

	Sources []Source `hcl:"source,block"`
}

type Source struct {
	Type string `hcl:"type,label"`

	Body hcl2.Body `hcl:",remain"`
}

type SourceFile struct {
	Path    string  `hcl:"path,attr"`
	Default *string `hcl:"default,attr"`
	Key     *string `hcl:"key,attr"`
}

type Step struct {
	Name string `hcl:"name,label"`

	Run StaticRun `hcl:"run,block"`

	Needs *[]string `hcl:"need,attr"`
}

type Exec struct {
	Command hcl2.Expression `hcl:"command,attr"`

	Args hcl2.Expression `hcl:"args,attr"`
	Env  hcl2.Expression `hcl:"env,attr"`
	Dir  hcl2.Expression `hcl:"dir,attr"`

	Interactive *bool `hcl:"interactive,attr"`
}

type DependsOn struct {
	Name string `hcl:"name,label"`

	Items hcl2.Expression `hcl:"items,attr"`
	Args  hcl2.Expression `hcl:"args,attr"`
}

type LazyStaticRun struct {
	Run StaticRun `hcl:"run,block"`
}

type StaticRun struct {
	Name string `hcl:"name,label"`

	Args map[string]hcl2.Expression `hcl:",remain"`
}

type LazyDynamicRun struct {
	Run DynamicRun `hcl:"run,block"`
}

type DynamicRun struct {
	Job  string          `hcl:"job,attr"`
	Args hcl2.Expression `hcl:"with,attr"`
}

type Parameter struct {
	Name string `hcl:"name,label"`

	Type    hcl2.Expression `hcl:"type,attr"`
	Default hcl2.Expression `hcl:"default,attr"`
	Envs    []EnvSource     `hcl:"env,block"`

	Description *string `hcl:"description,attr"`
}

type EnvSource struct {
	Name string `hcl:"name,label"`
}

type SourceJob struct {
	Name string `hcl:"name,attr"`
	// This results in "no cty.Type for hcl.Expression" error
	//Arguments map[string]hcl2.Expression `hcl:"args,attr"`
	Args   hcl2.Expression `hcl:"args,attr"`
	Format *string         `hcl:"format,attr"`
	Key    *string         `hcl:"key,attr"`
}

type OptionSpec struct {
	Name string `hcl:"name,label"`

	Type        hcl2.Expression `hcl:"type,attr"`
	Default     hcl2.Expression `hcl:"default,attr"`
	Description *string         `hcl:"description,attr"`
	Short       *string         `hcl:"short,attr"`
}

type Variable struct {
	Name string `hcl:"name,label"`

	Type  hcl2.Expression `hcl:"type,attr"`
	Value hcl2.Expression `hcl:"value,attr"`
}

type JobSpec struct {
	//Type string `hcl:"type,label"`
	Name string `hcl:"name,label"`

	Version *string `hcl:"version,attr"`

	Module hcl2.Expression `hcl:"module,attr"`

	Description *string      `hcl:"description,attr"`
	Parameters  []Parameter  `hcl:"parameter,block"`
	Options     []OptionSpec `hcl:"option,block"`
	Configs     []Config     `hcl:"config,block"`
	Secrets     []Config     `hcl:"secret,block"`
	Variables   []Variable   `hcl:"variable,block"`

	Concurrency hcl2.Expression `hcl:"concurrency,attr"`

	SourceLocator hcl2.Expression `hcl:"__source_locator,attr"`

	Deps   []DependsOn     `hcl:"depends_on,block"`
	Exec   *Exec           `hcl:"exec,block"`
	Assert []Assert        `hcl:"assert,block"`
	Fail   hcl2.Expression `hcl:"fail,attr"`
	Import *string         `hcl:"import,attr"`

	// Private hides the job from `variant run -h` when set to true
	Private *bool `hcl:"private,attr"`

	Log *LogSpec `hcl:"log,block"`

	Steps []Step `hcl:"step,block"`

	Body hcl2.Body `hcl:",remain"`
}

type LogSpec struct {
	File     hcl2.Expression `hcl:"file,attr"`
	Stream   hcl2.Expression `hcl:"stream,attr"`
	Collects []Collect       `hcl:"collect,block"`
	Forwards []Forward       `hcl:"forward,block"`
}

type Collect struct {
	Condition hcl2.Expression `hcl:"condition,attr"`
	Format    hcl2.Expression `hcl:"format,attr"`
}

type Forward struct {
	Run *StaticRun `hcl:"run,block"`
}

type Assert struct {
	Name string `hcl:"name,label"`

	Condition hcl2.Expression `hcl:"condition,attr"`
}

type HCL2Config struct {
	Jobs    []JobSpec `hcl:"job,block"`
	Tests   []Test    `hcl:"test,block"`
	JobSpec `hcl:",remain"`
}

type Test struct {
	Name string `hcl:"name,label"`

	Variables []Variable `hcl:"variable,block"`
	Cases     []Case     `hcl:"case,block"`
	Run       StaticRun  `hcl:"run,block"`
	Assert    []Assert   `hcl:"assert,block"`

	SourceLocator hcl2.Expression `hcl:"__source_locator,attr"`
}

type Case struct {
	Name string `hcl:"name,label"`

	Args map[string]hcl2.Expression `hcl:",remain"`
}

func (t *configurable) HCL2Config() (*HCL2Config, error) {
	config := &HCL2Config{}

	ctx := &hcl2.EvalContext{
		Functions: conf.Functions("."),
		Variables: map[string]cty.Value{
			"name": cty.StringVal("Ermintrude"),
			"age":  cty.NumberIntVal(32),
			"path": cty.ObjectVal(map[string]cty.Value{
				"root":    cty.StringVal("rootDir"),
				"module":  cty.StringVal("moduleDir"),
				"current": cty.StringVal("currentDir"),
			}),
		},
	}

	diags := gohcl2.DecodeBody(t.Body, ctx, config)
	if diags.HasErrors() {
		return config, diags
	}

	return config, nil
}

type App struct {
	BinName string

	Files     map[string]*hcl2.File
	Config    *HCL2Config
	JobByName map[string]JobSpec

	Stdout, Stderr io.Writer

	Trace string
}

func New(dir string) (*App, error) {
	nameToFiles, cc, err := newConfigFromDir(dir)

	app := &App{
		Files: nameToFiles,
		Trace: os.Getenv("VARIANT_TRACE"),
	}

	if err != nil {
		return app, err
	}

	return newApp(app, cc, dir, true)
}

func NewFromFile(file string) (*App, error) {
	nameToFiles, cc, err := newConfigFromFiles([]string{file})

	app := &App{
		Files: nameToFiles,
	}

	if err != nil {
		return app, err
	}

	dir := filepath.Dir(file)

	return newApp(app, cc, dir, true)
}

func NewFromSources(srcs map[string][]byte) (*App, error) {
	nameToFiles, _, cc, err := newConfigFromSources(srcs)

	app := &App{
		Files: nameToFiles,
	}

	if err != nil {
		return app, err
	}

	return newApp(app, cc, "", false)
}

func newConfigFromDir(dirPathOrURL string) (map[string]*hcl2.File, *HCL2Config, error) {
	var dir string

	s := strings.Split(dirPathOrURL, "::")

	if len(s) > 1 {
		forcePrefix := s[0]

		u, err := url.Parse(s[1])
		if err != nil {
			return nil, nil, err
		}

		remote, err := depresolver.New(depresolver.Home(".variant2/cache"))
		if err != nil {
			return nil, nil, err
		}

		us := forcePrefix + "::" + u.String()

		cacheDir, err := remote.ResolveDir(us)
		if err != nil {
			return nil, nil, err
		}

		dir = cacheDir
	} else {
		dir = dirPathOrURL
	}

	files, err := conf.FindVariantFiles(dir)
	if err != nil {
		return map[string]*hcl2.File{}, nil, fmt.Errorf("failed to get %s files: %v", conf.VariantFileExt, err)
	}

	return newConfigFromFiles(files)
}

func newConfigFromFiles(files []string) (map[string]*hcl2.File, *HCL2Config, error) {
	l := &hcl2Loader{
		Parser: hcl2parse.NewParser(),
	}

	c, nameToFiles, err := l.loadFile(files...)

	if err != nil {
		return nameToFiles, nil, err
	}

	cc, err := c.HCL2Config()

	return nameToFiles, cc, err
}

func newConfigFromSources(srcs map[string][]byte) (map[string]*hcl2.File, hcl2.Body, *HCL2Config, error) {
	l := &hcl2Loader{
		Parser: hcl2parse.NewParser(),
	}

	c, nameToFiles, err := l.loadSources(srcs)

	if err != nil {
		var body hcl2.Body
		if c != nil {
			body = c.Body
		}

		return nameToFiles, body, nil, err
	}

	cc, err := c.HCL2Config()

	return nameToFiles, c.Body, cc, err
}

func newApp(app *App, cc *HCL2Config, importBaseDir string, enableImports bool) (*App, error) {
	jobs := append([]JobSpec{cc.JobSpec}, cc.Jobs...)

	var conf *HCL2Config

	jobByName := map[string]JobSpec{}
	for _, j := range jobs {
		jobByName[j.Name] = j

		if j.Import != nil {
			if !enableImports {
				return nil, fmt.Errorf("[bug] Imports are disable in the embedded mode")
			}

			var d string

			if strings.Contains(*j.Import, ":") {
				d = *j.Import
			} else {
				d = filepath.Join(importBaseDir, *j.Import)
			}

			a, err := New(d)

			if err != nil {
				return nil, err
			}

			importedJobs := append([]JobSpec{a.Config.JobSpec}, a.Config.Jobs...)
			for _, importedJob := range importedJobs {
				var newJobName string
				if j.Name == "" {
					newJobName = importedJob.Name
				} else {
					newJobName = fmt.Sprintf("%s %s", j.Name, importedJob.Name)
				}

				jobByName[newJobName] = importedJob

				if j.Name == "" && importedJob.Name == "" {
					conf = a.Config
				}
			}
		}
	}

	if conf == nil {
		conf = cc
	}

	app.Config = conf

	app.JobByName = jobByName

	return app, nil
}

func (app *App) Run(cmd string, args map[string]interface{}, opts map[string]interface{}, fs ...SetOptsFunc) (*Result, error) {
	var f SetOptsFunc
	if len(fs) > 0 {
		f = fs[0]
	}

	jr, err := app.Job(nil, cmd, args, opts, f)
	if err != nil {
		return nil, err
	}

	return jr()
}

func (app *App) run(l *EventLogger, cmd string, args map[string]interface{}, opts map[string]interface{}) (*Result, error) {
	jr, err := app.Job(l, cmd, args, opts, nil)
	if err != nil {
		return nil, err
	}

	return jr()
}

func (app *App) Job(l *EventLogger, cmd string, args map[string]interface{}, opts map[string]interface{}, f SetOptsFunc) (func() (*Result, error), error) {
	jobByName := app.JobByName

	j, ok := jobByName[cmd]
	if !ok {
		j, ok = jobByName[""]
		if !ok {
			return nil, fmt.Errorf("command %q not found", cmd)
		}
	}

	return func() (*Result, error) {
		cc := app.Config

		jobCtx, err := app.createJobContext(cc, j, args, opts, f)

		if err != nil {
			app.PrintError(err)
			return nil, err
		}

		jobEvalCtx := jobCtx.evalContext

		if l == nil {
			l = NewEventLogger(cmd, args, opts)
			l.Stderr = app.Stderr

			if app.Trace != "" {
				l.Register(app.newTracingLogCollector())
			}
		}

		if j.Log != nil {
			if len(j.Log.Collects) == 0 {
				return nil, fmt.Errorf("log config for job %q is invalid: at least one collect block is required", j.Name)
			}

			var file string

			if nonEmptyExpression(j.Log.File) {
				if diags := gohcl2.DecodeExpression(j.Log.File, jobEvalCtx, &file); diags.HasErrors() {
					app.PrintDiags(diags)
					return nil, diags
				}
			}

			logCollector := app.newLogCollector(file, j, jobCtx)
			unregister := l.Register(logCollector)

			defer func() {
				if err := unregister(); err != nil {
					panic(err)
				}
			}()

			{
				var stream string

				if nonEmptyExpression(j.Log.Stream) {
					if diags := gohcl2.DecodeExpression(j.Log.Stream, jobEvalCtx, &stream); diags.HasErrors() {
						app.PrintDiags(diags)
						return nil, diags
					}
				}

				if stream != "" {
					l.Stream = stream
				}
			}
		}

		conf, err := app.getConfigs(jobEvalCtx, cc, j, "config", func(j JobSpec) []Config { return j.Configs }, nil)
		if err != nil {
			return nil, err
		}

		jobEvalCtx.Variables["conf"] = conf

		secretRefsEvaluator, err := vals.New(vals.Options{CacheSize: 100})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize vals: %v", err)
		}

		sec, err := app.getConfigs(jobEvalCtx, cc, j, "secret", func(j JobSpec) []Config { return j.Secrets }, func(m map[string]interface{}) (map[string]interface{}, error) {
			return secretRefsEvaluator.Eval(m)
		})

		if err != nil {
			return nil, err
		}

		jobEvalCtx.Variables["sec"] = sec

		needs := map[string]cty.Value{}

		var concurrency int

		if !IsExpressionEmpty(j.Concurrency) {
			if err := gohcl2.DecodeExpression(j.Concurrency, jobEvalCtx, &concurrency); err != nil {
				app.PrintDiags(err)
				return nil, err
			}

			if concurrency < 1 {
				return nil, fmt.Errorf("concurrency less than 1 can not be set. If you wanted %d for a concurrency equals to the number of steps, is isn't a good idea. Some system has a relatively lower fd limit that can make your command fail only when there are too many steps. Always use static number of concurrency", concurrency)
			}
		} else {
			concurrency = 1
		}

		{
			res, err := app.execJobSteps(l, jobCtx, needs, j.Steps, concurrency)
			if res != nil || err != nil {
				app.PrintDiags(err)
				return res, err
			}
		}

		{
			r, err := app.execJob(l, j, jobCtx)
			if r == nil && err == nil {
				return nil, fmt.Errorf(NoRunMessage)
			}
			app.PrintDiags(err)
			return r, err
		}
	}, nil
}

func (app *App) WriteDiags(diagnostics hcl2.Diagnostics) {
	wr := hcl2.NewDiagnosticTextWriter(
		os.Stderr, // writer to send messages to
		app.Files, // the parser's file cache, for source snippets
		100,       // wrapping width
		true,      // generate colored/highlighted output
	)
	if err := wr.WriteDiagnostic(diagnostics[0]); err != nil {
		panic(err)
	}
}

func (app *App) ExitWithError(err error) {
	app.PrintError(err)
	os.Exit(1)
}

func (app *App) PrintError(err error) {
	switch diags := err.(type) {
	case hcl2.Diagnostics:
		app.WriteDiags(diags)
	default:
		fmt.Fprintf(os.Stderr, "%v", err)
	}
}

func (app *App) PrintDiags(err error) {
	switch diags := err.(type) {
	case hcl2.Diagnostics:
		app.WriteDiags(diags)
	}
}

type Command struct {
	Name string
	Args []string
	Env  map[string]string
	Dir  string

	Interactive bool
}

func (app *App) execCmd(cmd Command, log bool) (*Result, error) {
	env := map[string]string{}

	// We need to explicitly inherit os envvars.
	// Otherwise the command is executed in an env that misses all of them, including the important one like PATH,
	// which is confusing to users.
	// Perhaps in the future, we could introduce a `exec` block attribute to optionally turn off the inheritance.
	for _, e := range os.Environ() {
		pair := strings.Split(e, "=")

		env[pair[0]] = pair[1]
	}

	for k, v := range cmd.Env {
		env[k] = v
	}

	shellCmd := &shell.Command{
		Name: cmd.Name,
		Args: cmd.Args,
		Env:  env,
		Dir:  cmd.Dir,
	}

	sh := shell.Shell{
		Exec: shell.DefaultExec,
	}

	var err error

	var re *Result

	if cmd.Interactive {
		shellCmd.Stdin = os.Stdin
		shellCmd.Stdout = os.Stdout
		shellCmd.Stderr = os.Stderr

		res := sh.Wait(shellCmd)

		err = res.Error

		re = &Result{}
	} else {
		var opts shell.CaptureOpts

		if log {
			opts.LogStdout = func(line string) {
				fmt.Fprintf(app.Stdout, "%s\n", line)
			}
			opts.LogStderr = func(line string) {
				fmt.Fprintf(app.Stderr, "%s\n", line)
			}
		}

		var res *shell.CaptureResult

		res, err = sh.Capture(shellCmd, opts)

		re = &Result{
			Stdout: res.Stdout,
			Stderr: res.Stderr,
		}
	}

	//nolint
	switch e := err.(type) {
	case *exec.ExitError:
		re.ExitStatus = e.ExitCode()
	}

	if err != nil {
		msg := app.sanitize(fmt.Sprintf("command \"%s %s\"", cmd.Name, strings.Join(cmd.Args, " ")))

		if cmd.Dir != "" {
			msg += fmt.Sprintf(" in %q", cmd.Dir)
		}

		if !log {
			msg += fmt.Sprintf(`
COMBINED OUTPUT:
%s`,
				text.Indent(re.Stdout+re.Stderr, "  "),
			)
		}

		return re, errors.Wrap(err, msg)
	}

	return re, nil
}

func (app *App) sanitize(str string) string {
	return str
}

func (app *App) execJob(l *EventLogger, j JobSpec, jobCtx *JobContext) (*Result, error) {
	var res *Result

	var err error

	var cmd string

	var args []string

	var env map[string]string

	var dir string

	var depStdout string

	var lastDepRes *Result

	evalCtx := jobCtx.evalContext

	if j.Deps != nil {
		for i := range j.Deps {
			d := j.Deps[i]

			var err error

			lastDepRes, err = app.execMultiRun(l, evalCtx, &d)
			if err != nil {
				return nil, err
			}

			depStdout += lastDepRes.Stdout
		}
	}

	if j.Exec != nil {
		if diags := gohcl2.DecodeExpression(j.Exec.Command, evalCtx, &cmd); diags.HasErrors() {
			return nil, diags
		}

		if diags := gohcl2.DecodeExpression(j.Exec.Args, evalCtx, &args); diags.HasErrors() {
			return nil, diags
		}

		if diags := gohcl2.DecodeExpression(j.Exec.Env, evalCtx, &env); diags.HasErrors() {
			return nil, diags
		}

		if !IsExpressionEmpty(j.Exec.Dir) {
			if diags := gohcl2.DecodeExpression(j.Exec.Dir, evalCtx, &dir); diags.HasErrors() {
				return nil, diags
			}
		}

		c := Command{
			Name: cmd,
			Args: args,
			Env:  env,
			Dir:  dir,
		}

		if j.Exec.Interactive != nil && *j.Exec.Interactive {
			c.Interactive = true
		}

		res, err = app.execCmd(c, true)
		if err := l.LogExec(cmd, args); err != nil {
			return nil, err
		}
	} else {
		either := eitherJobRun{}

		var lazyDynamicRun LazyDynamicRun

		var lazyStaticRun LazyStaticRun

		sErr := gohcl2.DecodeBody(j.Body, evalCtx, &lazyStaticRun)

		if sErr.HasErrors() {
			dErr := gohcl2.DecodeBody(j.Body, evalCtx, &lazyDynamicRun)

			if dErr != nil {
				sErrMsg := sErr.Error()
				if !strings.Contains(sErrMsg, "Missing run block") && !strings.Contains(sErrMsg, "Missing name for run") {
					return nil, sErr
				}

				dErrMsg := dErr.Error()
				if !strings.Contains(dErrMsg, "Missing run block") {
					return nil, dErr
				}
			} else {
				either.dynamic = &lazyDynamicRun.Run
			}
		} else {
			either.static = &lazyStaticRun.Run
		}

		if either.static != nil || either.dynamic != nil {
			res, err = app.runJobAndUpdateContext(l, jobCtx, either, new(sync.Mutex))
		} else if j.Assert != nil {
			for _, a := range j.Assert {
				if err2 := app.execAssert(evalCtx, a); err2 != nil {
					return nil, err2
				}
			}
			return &Result{}, nil
		}
	}

	if j.Assert != nil && len(j.Assert) > 0 {
		for _, a := range j.Assert {
			if err2 := app.execAssert(evalCtx, a); err2 != nil {
				return nil, err2
			}
		}
	}

	if res == nil {
		// The job contained only `depends_on` block(s)
		// Treat the result of depends_on as the result of this job
		if lastDepRes != nil {
			lastDepRes.Stdout = depStdout

			return lastDepRes, nil
		}

		// The job had no operation
		return res, nil
	}

	// The job contained job or step(s).
	// If we also had depends_on block(s), concat all the results
	if depStdout != "" {
		res.Stdout = depStdout + res.Stdout
	}

	return res, err
}

func (app *App) execAssert(ctx *hcl2.EvalContext, a Assert) error {
	var assert bool

	cond := a.Condition

	diags := gohcl2.DecodeExpression(cond, ctx, &assert)
	if diags.HasErrors() {
		return diags
	}

	if !assert {
		fp, err := os.Open(cond.Range().Filename)
		if err != nil {
			panic(err)
		}
		defer fp.Close()

		start := cond.Range().Start.Byte
		b, err := ioutil.ReadAll(fp)

		if err != nil {
			panic(err)
		}

		last := cond.Range().End.Byte + 1
		expr := b[start:last]

		traversals := cond.Variables()
		vars := []string{}

		for _, t := range traversals {
			ctyValue, err := t.TraverseAbs(ctx)
			if err == nil {
				v, err := ctyToGo(ctyValue)
				if err != nil {
					panic(err)
				}

				src := strings.TrimSpace(string(b[t.SourceRange().Start.Byte:t.SourceRange().End.Byte]))
				vars = append(vars, fmt.Sprintf("%s=%v (%T)", src, v, v))
			}
		}

		return fmt.Errorf("assertion %q failed: this expression must be true, but was false: %s, where %s", a.Name, expr, strings.Join(vars, " "))
	}

	return nil
}

func failOnPanic(t *testing.T) {
	r := recover()
	if r != nil {
		t.Errorf("test panicked: %v\n%s", r, debug.Stack())
		t.FailNow()
	}
}
func (app *App) RunTests(pat string) (*Result, error) {
	var res *Result

	var suite []testing.InternalTest

	for i := range app.Config.Tests {
		test := app.Config.Tests[i]

		suite = append(suite, testing.InternalTest{
			Name: rewrite(test.Name),
			F: func(t *testing.T) {
				defer failOnPanic(t)
				app.execTest(t, test)
			},
		})
	}

	main := testing.MainStart(TestDeps{}, suite, nil, nil)

	if err := flag.Set("test.run", rewrite(pat)); err != nil {
		panic(err)
	}
	// Avoid error like this:
	//   testing: can't write /var/folders/lx/53d8_kgd26vf5_drrg89wkvc0000gp/T/go-build584494014/b001/testlog.txt: close /var/folders/lx/53d8_kgd26vf5_drrg89wkvc0000gp/T/go-build584494014/b001/testlog.txt: file already closed
	if err := flag.Set("test.testlogfile", ""); err != nil {
		panic(err)
	}

	code := main.Run()
	if code != 0 {
		return nil, fmt.Errorf("test exited with code %d", code)
	}

	return res, nil
}

func (app *App) execTest(t *testing.T, test Test) *Result {
	var cases []Case

	if len(test.Cases) == 0 {
		cases = []Case{{}}
	} else {
		cases = test.Cases
	}

	var res *Result

	for i := range cases {
		c := cases[i]
		t.Run(c.Name, func(t *testing.T) {
			var err error
			res, err = app.execTestCase(test, c)
			if err != nil {
				app.PrintError(err)
				t.Fatalf("%v", err)
			}
		})
	}

	return res
}

func (app *App) execTestCase(t Test, c Case) (*Result, error) {
	ctx := &hcl2.EvalContext{
		Functions: conf.Functions("."),
		Variables: map[string]cty.Value{
			"context": getContext(t.SourceLocator),
		},
	}

	ctx, err := addVariables(ctx, t.Variables)
	if err != nil {
		return nil, err
	}

	caseFields := map[string]cty.Value{}

	for k, expr := range c.Args {
		var v cty.Value
		if diags := gohcl2.DecodeExpression(expr, ctx, &v); diags.HasErrors() {
			return nil, diags
		}

		caseFields[k] = v
	}

	caseVal := cty.ObjectVal(caseFields)
	ctx.Variables["case"] = caseVal

	jobCtx := &JobContext{
		evalContext: ctx,
		globalArgs:  map[string]interface{}{},
	}

	res, err := app.runJobAndUpdateContext(nil, jobCtx, eitherJobRun{static: &t.Run}, new(sync.Mutex))

	if res == nil && err != nil {
		return nil, err
	}

	// If there are one ore more assert(s), do not fail immediately and let the assertion(s) decide
	if t.Assert != nil && len(t.Assert) > 0 {
		var lines []string

		for _, a := range t.Assert {
			if err := app.execAssert(ctx, a); err != nil {
				if strings.HasPrefix(err.Error(), "assertion \"") {
					return nil, fmt.Errorf("case %q: %v", c.Name, err)
				}

				return nil, err
			}

			lines = append(lines, fmt.Sprintf("PASS: %s", a.Name))
		}

		testReport := strings.Join(lines, "\n")

		return &Result{Stdout: testReport}, nil
	}

	return res, err
}

type Result struct {
	Stdout     string
	Stderr     string
	Noop       bool
	ExitStatus int
}

func (res *Result) toCty() cty.Value {
	if res == nil {
		return cty.ObjectVal(map[string]cty.Value{
			"stdout":     cty.StringVal("<not set>"),
			"stderr":     cty.StringVal("<not set>>"),
			"exitstatus": cty.NumberIntVal(int64(-127)),
			"set":        cty.BoolVal(false),
		})
	}

	return cty.ObjectVal(map[string]cty.Value{
		"stdout":     cty.StringVal(res.Stdout),
		"stderr":     cty.StringVal(res.Stderr),
		"exitstatus": cty.NumberIntVal(int64(res.ExitStatus)),
		"set":        cty.BoolVal(true),
	})
}

func (app *App) execRunInternal(l *EventLogger, jobCtx *JobContext, run eitherJobRun) (*Result, error) {
	var jobRun *jobRun

	var err error

	if run.static != nil {
		jobRun, err = staticRunToJob(jobCtx, run.static)

		if err != nil {
			return nil, err
		}
	} else {
		jobRun, err = dynamicRunToJob(jobCtx, run.dynamic)

		if err != nil {
			return nil, err
		}
	}

	return app.execRunArgs(l, jobRun.Name, jobRun.Args)
}

func (app *App) execRunArgs(l *EventLogger, name string, args map[string]interface{}) (*Result, error) {
	if l != nil {
		if err := l.LogRun(name, args); err != nil {
			return nil, err
		}
	}

	return app.run(l, name, args, args)
}

func cloneEvalContext(c *hcl2.EvalContext) *hcl2.EvalContext {
	var ctx hcl2.EvalContext

	ctx.Variables = map[string]cty.Value{}

	for k, v := range c.Variables {
		ctx.Variables[k] = v
	}

	ctx.Functions = map[string]function.Function{}

	for k, v := range c.Functions {
		ctx.Functions[k] = v
	}

	return &ctx
}

func (app *App) execMultiRun(l *EventLogger, jobCtx *hcl2.EvalContext, r *DependsOn) (*Result, error) {
	ctx := cloneEvalContext(jobCtx)

	ctyItems := []cty.Value{}

	items := []interface{}{}

	if !IsExpressionEmpty(r.Items) {
		if err := gohcl2.DecodeExpression(r.Items, jobCtx, &ctyItems); err != nil {
			return nil, err
		}

		for _, item := range ctyItems {
			v, err := ctyToGo(item)

			if err != nil {
				return nil, err
			}

			items = append(items, v)
		}
	}

	if len(items) > 0 {
		var stdout string

		for _, item := range items {
			iterCtx := cloneEvalContext(ctx)

			v, err := goToCty(item)
			if err != nil {
				return nil, err
			}

			iterCtx.Variables["item"] = v

			args, err := exprToGoMap(iterCtx, r.Args)

			if err != nil {
				return nil, err
			}

			res, err := app.execRunArgs(l, r.Name, args)

			if err != nil {
				return res, err
			}

			stdout += res.Stdout + "\n"
		}

		return &Result{
			Stdout:     stdout,
			Stderr:     "",
			Noop:       false,
			ExitStatus: 0,
		}, nil
	}

	args := map[string]interface{}{}

	ctyArgs := map[string]cty.Value{}

	if err := gohcl2.DecodeExpression(r.Args, ctx, &ctyArgs); err != nil {
		return nil, err
	}

	for k, v := range ctyArgs {
		var err error

		args[k], err = ctyToGo(v)

		if err != nil {
			return nil, err
		}
	}

	res, err := app.execRunArgs(l, r.Name, args)
	if err != nil {
		return res, err
	}

	res.Stdout += "\n"

	return res, nil
}

func (app *App) runJobAndUpdateContext(l *EventLogger, jobCtx *JobContext, run eitherJobRun, m sync.Locker) (*Result, error) {
	res, err := app.execRunInternal(l, jobCtx, run)

	if res == nil {
		res = &Result{ExitStatus: 1, Stderr: err.Error()}
	}

	m.Lock()
	defer m.Unlock()

	runFields := map[string]cty.Value{}
	runFields["res"] = res.toCty()

	if err != nil {
		runFields["err"] = cty.StringVal(err.Error())
	} else {
		runFields["err"] = cty.StringVal("")
	}

	runVal := cty.ObjectVal(runFields)
	jobCtx.evalContext.Variables["run"] = runVal

	return res, err
}

func (app *App) execJobSteps(l *EventLogger, jobCtx *JobContext, results map[string]cty.Value, steps []Step, concurrency int) (*Result, error) {
	stepEvalCtx := *jobCtx.evalContext

	vars := map[string]cty.Value{}
	for k, v := range stepEvalCtx.Variables {
		vars[k] = v
	}

	stepEvalCtx.Variables = vars

	stepCtx := *jobCtx
	stepCtx.evalContext = &stepEvalCtx

	m := new(sync.Mutex)

	idToF := map[string]func() (*Result, error){}

	var dagNodeIds []string

	dagNodeIDToDeps := map[string][]string{}
	dagNodeIDToIndex := map[string]int{}

	var lastRes *Result

	for i := range steps {
		s := steps[i]

		f := func() (*Result, error) {
			res, err := app.runJobAndUpdateContext(l, &stepCtx, eitherJobRun{static: &s.Run}, m)
			if err != nil {
				return res, err
			}

			m.Lock()

			results[s.Name] = res.toCty()
			resultsCty := cty.ObjectVal(results)
			stepEvalCtx.Variables["step"] = resultsCty

			m.Unlock()

			return res, nil
		}

		idToF[s.Name] = f

		dagNodeIDToIndex[s.Name] = i

		dagNodeIds = append(dagNodeIds, s.Name)

		if s.Needs != nil {
			dagNodeIDToDeps[s.Name] = *s.Needs
		} else {
			dagNodeIDToDeps[s.Name] = []string{}
		}
	}

	g := dag.New(dag.Nodes(dagNodeIds))

	for id, deps := range dagNodeIDToDeps {
		g.AddDependencies(id, deps)
	}

	plan, err := g.Plan()
	if err != nil {
		return nil, err
	}

	type result struct {
		r   *Result
		err error
	}

	var rs []result

	for _, nodes := range plan {
		ids := []string{}
		for _, n := range nodes {
			ids = append(ids, n.Id)
		}
		// Preserve the order of definitions
		sort.Slice(ids, func(i, j int) bool {
			return dagNodeIDToIndex[ids[i]] < dagNodeIDToIndex[ids[j]]
		})

		var wg sync.WaitGroup

		rs = make([]result, len(ids))

		var rsm sync.Mutex

		workqueue := make(chan func())

		for c := 0; c < concurrency; c++ {
			go func() {
				for w := range workqueue {
					w()
				}
			}()
		}

		for i := range ids {
			id := ids[i]

			f := idToF[id]

			ii := i

			wg.Add(1)
			workqueue <- func() {
				defer wg.Done()
				r, err := f()
				rsm.Lock()
				rs[ii] = result{r: r, err: err}
				rsm.Unlock()
			}
		}

		wg.Wait()

		lastRes = rs[len(rs)-1].r

		var rese *multierror.Error

		for i := range rs {
			if rs[i].err != nil {
				rese = multierror.Append(rese, err)
			}
		}

		if rese != nil && rese.Len() > 0 {
			return lastRes, rese
		}
	}

	if len(rs) > 0 {
		var sum Result

		sum.ExitStatus = lastRes.ExitStatus

		for i, r := range rs {
			if i != 0 {
				sum.Stdout += "\n"
				sum.Stderr += "\n"
			}

			sum.Stdout += r.r.Stdout
			sum.Stderr += r.r.Stderr
		}

		return &sum, nil
	}

	return lastRes, nil
}

func getContext(sourceLocator hcl2.Expression) cty.Value {
	sourcedir := cty.StringVal(filepath.Dir(sourceLocator.Range().Filename))
	context := map[string]cty.Value{}
	{
		context["sourcedir"] = sourcedir
	}

	ctx := cty.ObjectVal(context)

	return ctx
}

func getDefault(ctx cty.Value, def hcl2.Expression, tpe cty.Type) (*cty.Value, error) {
	r := def.Range()

	if r.Start != r.End {
		var vv cty.Value

		defCtx := &hcl2.EvalContext{
			Functions: conf.Functions("."),
			Variables: map[string]cty.Value{
				"context": ctx,
			},
		}
		if err := gohcl2.DecodeExpression(def, defCtx, &vv); err != nil {
			return nil, err
		}

		// Necessary for .variant.json support, where `r.Start != r.End` even for options and parameters without `default` attrs
		if vv.Type() == cty.DynamicPseudoType {
			return nil, nil
		}

		if !vv.Type().Equals(tpe) {
			// Supported automatic type conversions
			if tpe.Equals(cty.Map(cty.String)) && vv.Type().IsObjectType() {
				m := map[string]interface{}{}

				for k, v := range vv.AsValueMap() {
					switch v.Type() {
					case cty.String:
						m[k] = v.AsString()
					default:
						return nil, fmt.Errorf("unexpected type of value encountered while reading object. for %q, got %v(%s), wanted %s", k, v.GoString(), v.Type().FriendlyName(), "string")
					}
				}

				var err error

				vv, err = goToCty(m)

				if err != nil {
					return nil, err
				}
			} else {
				return nil, errors.WithStack(fmt.Errorf("unexpected type of value %v provided: want %s, got %s", vv, tpe.FriendlyName(), vv.Type().FriendlyName()))
			}
		}

		return &vv, nil
	}

	return nil, nil
}

func getValueFor(ctx cty.Value, name string, typeExpr hcl2.Expression, defaultExpr hcl2.Expression, provided map[string]interface{}) (*cty.Value, *cty.Type, error) {
	v := provided[name]

	tpe, diags := typeexpr.TypeConstraint(typeExpr)
	if diags != nil {
		return nil, nil, diags
	}

	switch v.(type) {
	case nil:
		vv, err := getDefault(ctx, defaultExpr, tpe)
		if err != nil {
			return nil, nil, err
		}

		return vv, &tpe, nil
	}

	//a, err := goToCty(v)
	//
	//if err != nil {
	//	return nil, nil, err
	//}
	//
	//if !a.Type().Equals(tpe) {
	//	return nil, nil, fmt.Errorf("unexpected type. want %q, got %q", tpe.FriendlyNameForConstraint(), a.Type().FriendlyName())
	//}

	//if vty, err := gocty.ImpliedType(v); err != nil {
	//	return nil, nil, err
	//} else if vty != tpe {
	//	return nil, nil, fmt.Errorf("unexpected type. want %q, got %q", tpe.FriendlyNameForConstraint(), vty.FriendlyName())
	//}

	val, err := gocty.ToCtyValue(v, tpe)
	if err != nil {
		return nil, nil, err
	}

	return &val, &tpe, nil
}

type JobContext struct {
	evalContext *hcl2.EvalContext

	globalArgs map[string]interface{}
}

func (app *App) createJobContext(cc *HCL2Config, j JobSpec, givenParams map[string]interface{}, givenOpts map[string]interface{}, f SetOptsFunc) (*JobContext, error) {
	ctx := getContext(j.SourceLocator)

	globalParams, err := setParameterValues("global parameter", ctx, cc.Parameters, givenParams)
	if err != nil {
		return nil, fmt.Errorf("job %q: %w", j.Name, err)
	}

	localParams, err := setParameterValues("parameter", ctx, j.Parameters, givenParams)
	if err != nil {
		return nil, fmt.Errorf("job %q: %w", j.Name, err)
	}

	params := map[string]cty.Value{}

	for k, v := range globalParams {
		params[k] = v
	}

	// In case this is not a default/root job, we have a separate set of parameters to override the globals. So:
	if j.Name != "" {
		for k, v := range localParams {
			if _, ok := params[k]; ok {
				return nil, fmt.Errorf("job %q: shadowing global parameter %q with parameter %q is not allowed", j.Name, k, k)
			}

			params[k] = v
		}
	}

	globalOpts, err := setOptionValues("global option", ctx, cc.Options, givenOpts, f)
	if err != nil {
		return nil, fmt.Errorf("job %q: %w", j.Name, err)
	}

	localOpts, err := setOptionValues("option", ctx, j.Options, givenOpts, f)
	if err != nil {
		return nil, fmt.Errorf("job %q: %w", j.Name, err)
	}

	opts := map[string]cty.Value{}

	for k, v := range globalOpts {
		opts[k] = v
	}

	// In case this is not a default/root job, we have a separate set of options to override the globals. So:
	if j.Name != "" {
		for k, v := range localOpts {
			if _, ok := opts[k]; ok {
				return nil, fmt.Errorf("job %q: shadowing global option %q with option %q is not allowed", j.Name, k, k)
			}

			opts[k] = v
		}
	}

	varSpecs := append(append([]Variable{}, cc.Variables...), j.Variables...)
	varCtx := &hcl2.EvalContext{
		Functions: conf.Functions("."),
		Variables: map[string]cty.Value{
			"param":   cty.ObjectVal(params),
			"opt":     cty.ObjectVal(opts),
			"context": ctx,
		},
	}

	jobCtx, err := addVariables(varCtx, varSpecs)
	if err != nil {
		return nil, err
	}

	mod, err := getModule(jobCtx, cc.Module, j.Module)
	if err != nil {
		return nil, err
	}

	jobCtx.Variables["mod"] = mod

	globalArgs := map[string]interface{}{}

	for k, p := range globalParams {
		v, err := ctyToGo(p)
		if err != nil {
			return nil, fmt.Errorf("converting global parameter %q to go: %w", k, err)
		}

		globalArgs[k] = v
	}

	for k, o := range globalOpts {
		if _, ok := globalArgs[k]; ok {
			return nil, fmt.Errorf("shadowing parameter %q with option %q is not allowed", k, k)
		}

		v, err := ctyToGo(o)
		if err != nil {
			return nil, fmt.Errorf("converting global option %q to go: %w", k, err)
		}

		globalArgs[k] = v
	}

	return &JobContext{
		evalContext: jobCtx,
		globalArgs:  globalArgs,
	}, nil
}

func (app *App) getConfigs(confCtx *hcl2.EvalContext, cc *HCL2Config, j JobSpec, confType string, f func(JobSpec) []Config, g func(map[string]interface{}) (map[string]interface{}, error)) (cty.Value, error) {
	confSpecs := append(append([]Config{}, f(cc.JobSpec)...), f(j)...)

	confFields := map[string]cty.Value{}

	for confIndex := range confSpecs {
		confSpec := confSpecs[confIndex]
		merged := map[string]interface{}{}

		for sourceIdx := range confSpec.Sources {
			sourceSpec := confSpec.Sources[sourceIdx]

			var yamlData []byte

			var format string

			var key string

			switch sourceSpec.Type {
			case "file":
				var source SourceFile
				if err := gohcl2.DecodeBody(sourceSpec.Body, confCtx, &source); err != nil {
					return cty.NilVal, err
				}

				var err error

				yamlData, err = ioutil.ReadFile(source.Path)
				if err != nil {
					if source.Default == nil {
						return cty.NilVal, fmt.Errorf("job %q: %s %q: source %d: %v", j.Name, confType, confSpec.Name, sourceIdx, err)
					}

					yamlData = []byte(*source.Default)
				}

				format = FormatYAML

				if source.Key != nil {
					key = *source.Key
				}
			case "job":
				var source SourceJob
				if err := gohcl2.DecodeBody(sourceSpec.Body, confCtx, &source); err != nil {
					return cty.NilVal, err
				}

				args, err := exprToGoMap(confCtx, source.Args)
				if err != nil {
					return cty.NilVal, err
				}

				res, err := app.run(nil, source.Name, args, args)
				if err != nil {
					return cty.NilVal, err
				}

				yamlData = []byte(res.Stdout)

				if source.Format != nil {
					format = *source.Format
				} else {
					format = FormatYAML
				}

				if source.Key != nil {
					key = *source.Key
				}
			default:
				return cty.DynamicVal, fmt.Errorf("config source %q is not implemented. It must be either \"file\" or \"job\", so that it looks like `source file {` or `source file {`", sourceSpec.Type)
			}

			m := map[string]interface{}{}

			switch format {
			case FormatYAML:
				if err := yaml.Unmarshal(yamlData, &m); err != nil {
					return cty.NilVal, err
				}
			case FormatText:
				if key == "" {
					return cty.NilVal, fmt.Errorf("`key` must be specified for `text`-formatted source at %d", sourceIdx)
				}

				keys := strings.Split(key, ".")
				lastKeyIndex := len(keys) - 1
				intermediateKeys := keys[0:lastKeyIndex]
				lastKey := keys[lastKeyIndex]

				cur := m

				for _, k := range intermediateKeys {
					if _, ok := cur[k]; !ok {
						cur[k] = map[string]interface{}{}
					}
				}

				cur[lastKey] = string(yamlData)
			default:
				return cty.NilVal, fmt.Errorf("format %q is not implemented yet. It must be \"yaml\" or omitted", format)
			}

			if err := mergo.Merge(&merged, m, mergo.WithOverride); err != nil {
				return cty.NilVal, err
			}
		}

		if g != nil {
			r, err := g(merged)
			if err != nil {
				return cty.NilVal, err
			}

			merged = r
		}

		yamlData, err := yaml.Marshal(merged)
		if err != nil {
			return cty.NilVal, err
		}

		ty, err := ctyyaml.ImpliedType(yamlData)
		if err != nil {
			return cty.DynamicVal, err
		}

		v, err := ctyyaml.Unmarshal(yamlData, ty)
		if err != nil {
			return cty.DynamicVal, err
		}

		confFields[confSpec.Name] = v
	}

	return cty.ObjectVal(confFields), nil
}

func addVariables(varCtx *hcl2.EvalContext, varSpecs []Variable) (*hcl2.EvalContext, error) {
	varFields := map[string]cty.Value{}

	for _, varSpec := range varSpecs {
		var tpe cty.Type

		tv, _ := varSpec.Type.Value(nil)

		if !tv.IsNull() {
			var diags hcl2.Diagnostics

			tpe, diags = typeexpr.TypeConstraint(varSpec.Type)
			if diags != nil {
				return varCtx, diags
			}
		}

		if tpe.IsListType() && tpe.ListElementType().Equals(cty.String) {
			var v []string
			if err := gohcl2.DecodeExpression(varSpec.Value, varCtx, &v); err != nil {
				return varCtx, err
			}

			if vty, err := gocty.ImpliedType(v); err != nil {
				return varCtx, err
			} else if vty != tpe {
				return varCtx, fmt.Errorf("unexpected type of option. want %q, got %q", tpe.FriendlyNameForConstraint(), vty.FriendlyName())
			}

			val, err := gocty.ToCtyValue(v, tpe)
			if err != nil {
				return varCtx, err
			}

			varFields[varSpec.Name] = val
		} else {
			var v cty.Value

			if err := gohcl2.DecodeExpression(varSpec.Value, varCtx, &v); err != nil {
				return varCtx, err
			}

			vty := v.Type()

			if !tv.IsNull() && !vty.Equals(tpe) {
				return varCtx, fmt.Errorf("unexpected type of value for variable. want %q, got %q", tpe.FriendlyNameForConstraint(), vty.FriendlyName())
			}

			varFields[varSpec.Name] = v
		}

		varCtx.Variables["var"] = cty.ObjectVal(varFields)
	}

	return varCtx, nil
}

func nonEmptyExpression(x hcl2.Expression) bool {
	if x.Range().Start == x.Range().End {
		return false
	}

	v, errs := x.Value(nil)
	if errs != nil {
		return true
	}

	return v.Type() != cty.DynamicPseudoType
}

func getModule(ctx *hcl2.EvalContext, m1, m2 hcl2.Expression) (cty.Value, error) {
	var m hcl2.Expression

	if nonEmptyExpression(m2) {
		m = m2
	} else if nonEmptyExpression(m1) {
		m = m1
	} else {
		return cty.NilVal, nil
	}

	var moduleName string
	if err := gohcl2.DecodeExpression(m, ctx, &moduleName); err != nil {
		return cty.NilVal, err
	}

	fname := m.Range().Filename
	mod, err := variantmod.New(
		variantmod.ModuleFile(fmt.Sprintf("%s.variantmod", moduleName)),
		variantmod.LockFile(fmt.Sprintf("%s.variantmod.lock", moduleName)),
		variantmod.WD(filepath.Dir(fname)),
	)

	if err != nil {
		return cty.NilVal, err
	}

	_, err = mod.Build()
	if err != nil {
		return cty.NilVal, err
	}

	dirs, err := mod.ExecutableDirs()
	if err != nil {
		return cty.NilVal, err
	}

	pathAddition := strings.Join(dirs, ":")

	return cty.MapVal(map[string]cty.Value{
		"pathaddition": cty.StringVal(pathAddition),
	}), nil
}

func IsExpressionEmpty(ex hcl2.Expression) bool {
	return !nonEmptyExpression(ex)
}
