package app

import (
	"flag"
	"fmt"
	"github.com/hashicorp/go-multierror"
	hcl2 "github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
	hcl2parse "github.com/hashicorp/hcl/v2/hclparse"
	"github.com/imdario/mergo"
	"github.com/mumoshu/hcl2test/pkg/conf"
	"github.com/pkg/errors"
	"github.com/variantdev/dag/pkg/dag"
	"github.com/variantdev/mod/pkg/shell"
	"github.com/variantdev/mod/pkg/variantmod"
	"github.com/variantdev/vals"
	ctyyaml "github.com/zclconf/go-cty-yaml"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	"gopkg.in/yaml.v3"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"
)

const NoRunMessage = "nothing to run"

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
}

type Step struct {
	Name string `hcl:"name,label"`

	Run *RunJob `hcl:"run,block"`

	Needs *[]string `hcl:"need,attr"`
}

type Exec struct {
	Command hcl2.Expression `hcl:"command,attr"`

	Args hcl2.Expression `hcl:"args,attr"`
	Env  hcl2.Expression `hcl:"env,attr"`
}

type RunJob struct {
	Name string `hcl:"name,label"`

	Args map[string]hcl2.Expression `hcl:",remain"`
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

	Exec   *Exec           `hcl:"exec,block"`
	Assert []Assert        `hcl:"assert,block"`
	Fail   hcl2.Expression `hcl:"fail,attr"`
	Run    *RunJob         `hcl:"run,block"`
	Import *string         `hcl:"import,attr"`

	Log *LogSpec `hcl:"log,block"`

	Steps []Step `hcl:"step,block"`
}

type LogSpec struct {
	File     hcl2.Expression `hcl:"file,attr"`
	Collects []Collect       `hcl:"collect,block"`
	Forwards []Forward       `hcl:"forward,block"`
}

type Collect struct {
	Condition hcl2.Expression `hcl:"condition,attr"`
	Format    hcl2.Expression `hcl:"format,attr"`
}

type Forward struct {
	Run *RunJob `hcl:"run,block"`
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
	Run       RunJob     `hcl:"run,block"`
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
		// We return the diags as an implementation of error, which the
		// caller than then type-assert if desired to recover the individual
		// diagnostics.
		// FIXME: The current API gives us no way to return warnings in the
		// absence of any errors.
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

	TraceCommands []string
}

func New(dir string) (*App, error) {
	nameToFiles, _, cc, err := newConfigFromDir(dir)

	app := &App{
		Files: nameToFiles,
	}

	if err != nil {
		return app, err
	}

	return newApp(app, cc, dir, true)
}

func NewFromFile(file string) (*App, error) {
	nameToFiles, _, cc, err := newConfigFromFiles([]string{file})

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

func newConfigFromDir(dir string) (map[string]*hcl2.File, hcl2.Body, *HCL2Config, error) {
	files, err := conf.FindVariantFiles(dir)
	if err != nil {
		return map[string]*hcl2.File{}, nil, nil, fmt.Errorf("failed to get %s files: %v", conf.VariantFileExt, err)
	}

	return newConfigFromFiles(files)
}

func newConfigFromFiles(files []string) (map[string]*hcl2.File, hcl2.Body, *HCL2Config, error) {
	l := &hcl2Loader{
		Parser: hcl2parse.NewParser(),
	}

	nameToFiles := map[string]*hcl2.File{}

	c, nameToFiles, err := l.loadFile(files...)

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

func newConfigFromSources(srcs map[string][]byte) (map[string]*hcl2.File, hcl2.Body, *HCL2Config, error) {
	l := &hcl2Loader{
		Parser: hcl2parse.NewParser(),
	}

	nameToFiles := map[string]*hcl2.File{}

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

			d := filepath.Join(importBaseDir, *j.Import)
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

func (app *App) Run(cmd string, args map[string]interface{}, opts map[string]interface{}) (*Result, error) {
	return app.run(nil, cmd, args, opts)
}

func (app *App) run(l *EventLogger, cmd string, args map[string]interface{}, opts map[string]interface{}) (*Result, error) {
	jobByName := app.JobByName
	cc := app.Config

	j, ok := jobByName[cmd]
	if !ok {
		j, ok = jobByName[""]
		if !ok {
			panic(fmt.Errorf("command %q not found", cmd))
		}
	}
	jobCtx, err := createJobContext(cc, j, args, opts)
	if err != nil {
		app.PrintError(err)
		return nil, err
	}

	if l == nil {
		l = NewEventLogger(cmd, args, opts)
	}

	if j.Log != nil && j.Log.Collects != nil && j.Log.Forwards != nil && len(j.Log.Forwards) > 0 {
		var file string
		if j.Log.File.Range().Start != j.Log.File.Range().End {
			if diags := gohcl2.DecodeExpression(j.Log.File, jobCtx, &file); diags.HasErrors() {
				return nil, diags
			}
		}
		logCollector := LogCollector{
			FilePath: file,
			CollectFn: func(evt Event) (*string, bool, error) {
				condVars := map[string]cty.Value{}
				for k, v := range jobCtx.Variables {
					condVars[k] = v
				}
				condVars["event"] = evt.toCty()
				condCtx := jobCtx
				condCtx.Variables = condVars

				for _, c := range j.Log.Collects {
					var condVal cty.Value
					if diags := gohcl2.DecodeExpression(c.Condition, condCtx, &condVal); diags.HasErrors() {
						return nil, false, diags
					}
					vv, err := ctyToGo(condVal)
					if err != nil {
						return nil, false, err
					}

					b, ok := vv.(bool)
					if !ok {
						return nil, false, fmt.Errorf("unexpected type of condition value: want bool, got %T", vv)
					}

					if !b {
						continue
					}

					formatVars := map[string]cty.Value{}
					for k, v := range jobCtx.Variables {
						formatVars[k] = v
					}
					formatVars["event"] = evt.toCty()
					formatCtx := jobCtx
					formatCtx.Variables = condVars

					var formatVal cty.Value
					if diags := gohcl2.DecodeExpression(c.Format, formatCtx, &formatVal); diags.HasErrors() {
						return nil, false, diags
					}
					formatV, err := ctyToGo(formatVal)
					if err != nil {
						return nil, false, err
					}
					f, ok := formatV.(string)
					if !ok {
						return nil, false, fmt.Errorf("unexpected type of format value: want string, got %T", f)
					}

					return &f, true, nil
				}

				return nil, false, nil
			},
			ForwardFn: func(log Log) error {
				logCty := cty.MapVal(map[string]cty.Value{
					"file": cty.StringVal(log.File),
				})
				jobCtx.Variables["log"] = logCty

				for _, f := range j.Log.Forwards {
					_, err := app.execRunInternal(nil, jobCtx, f.Run)
					if err != nil {
						return err
					}
				}
				return nil
			},
		}
		unregister := l.Register(logCollector)
		defer unregister()
	}

	conf, err := app.getConfigs(jobCtx, cc, j, "config", func(j JobSpec) []Config { return j.Configs }, nil)
	if err != nil {
		return nil, err
	}
	jobCtx.Variables["conf"] = conf

	secretRefsEvaluator, err := vals.New(vals.Options{CacheSize: 100})
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize vals: %v", err)
	}
	sec, err := app.getConfigs(jobCtx, cc, j, "secret", func(j JobSpec) []Config { return j.Secrets }, func(m map[string]interface{}) (map[string]interface{}, error) {
		return secretRefsEvaluator.Eval(m)
	})
	if err != nil {
		return nil, err
	}
	jobCtx.Variables["sec"] = sec

	needs := map[string]cty.Value{}
	var concurrency int
	if !IsExpressionEmpty(j.Concurrency) {
		if err := gohcl2.DecodeExpression(j.Concurrency, jobCtx, &concurrency); err != nil {
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
			return res, err
		}
	}

	{
		r, err := app.execJob(l, j, jobCtx)
		if r == nil && err == nil {
			return nil, fmt.Errorf(NoRunMessage)
		}
		return r, err
	}
}

func (app *App) WriteDiags(diagnostics hcl2.Diagnostics) {
	wr := hcl2.NewDiagnosticTextWriter(
		os.Stderr, // writer to send messages to
		app.Files, // the parser's file cache, for source snippets
		100,       // wrapping width
		true,      // generate colored/highlighted output
	)
	wr.WriteDiagnostics(diagnostics)
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

func (app *App) execCmd(cmd string, args []string, env map[string]string, log bool) (*Result, error) {
	app.TraceCommands = append(app.TraceCommands, fmt.Sprintf("%s %s", cmd, strings.Join(args, " ")))

	shellCmd := &shell.Command{
		Name: cmd,
		Args: args,
		//Stdout: os.Stdout,
		//Stderr: os.Stderr,
		//Stdin:  os.Stdin,
		Env: env,
	}

	sh := shell.Shell{
		Exec: shell.DefaultExec,
	}

	var opts shell.CaptureOpts

	if log {
		opts.LogStdout = func(line string) {
			fmt.Fprintf(app.Stdout, "%s\n", line)
		}
		opts.LogStderr = func(line string) {
			fmt.Fprintf(app.Stderr, "%s\n", line)
		}
	}

	// TODO exec with the runner
	res, err := sh.Capture(shellCmd, opts)

	re := &Result{
		Stdout: res.Stdout,
		Stderr: res.Stderr,
	}

	switch e := err.(type) {
	case *exec.ExitError:
		re.ExitStatus = e.ExitCode()
	}

	if err != nil {
		return re, errors.Wrap(err, app.sanitize(fmt.Sprintf("command \"%s %s\"", cmd, strings.Join(args, " "))))
	}

	return re, nil
}

func (app *App) sanitize(str string) string {
	return str
}

func (app *App) execJob(l *EventLogger, j JobSpec, ctx *hcl2.EvalContext) (*Result, error) {
	var res *Result
	var err error

	var cmd string
	var args []string
	var env map[string]string
	if j.Exec != nil {
		if diags := gohcl2.DecodeExpression(j.Exec.Command, ctx, &cmd); diags.HasErrors() {
			return nil, diags
		}

		if diags := gohcl2.DecodeExpression(j.Exec.Args, ctx, &args); diags.HasErrors() {
			return nil, diags
		}

		if diags := gohcl2.DecodeExpression(j.Exec.Env, ctx, &env); diags.HasErrors() {
			return nil, diags
		}

		res, err = app.execCmd(cmd, args, env, true)
		l.LogExec(cmd, args)
	} else if j.Run != nil {
		res, err = app.execRun(l, ctx, j.Run, new(sync.Mutex))
	} else if j.Assert != nil {
		for _, a := range j.Assert {
			if err2 := app.execAssert(ctx, a); err2 != nil {
				return nil, err2
			}
		}
		return &Result{}, nil
	}

	if j.Assert != nil && len(j.Assert) > 0 {
		for _, a := range j.Assert {
			if err2 := app.execAssert(ctx, a); err2 != nil {
				return nil, err2
			}
		}
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
				vars = append(vars, fmt.Sprintf("%s=%v (%T)", string(src), v, v))
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
				var err error
				res, err = app.execTest(t, test)
				if err != nil {
					t.Fatalf("%v", err)
				}
			},
		})
	}
	main := testing.MainStart(TestDeps{}, suite, nil, nil)
	flag.Set("test.run", rewrite(pat))
	// Avoid error like this:
	//   testing: can't write /var/folders/lx/53d8_kgd26vf5_drrg89wkvc0000gp/T/go-build584494014/b001/testlog.txt: close /var/folders/lx/53d8_kgd26vf5_drrg89wkvc0000gp/T/go-build584494014/b001/testlog.txt: file already closed
	flag.Set("test.testlogfile", "")
	code := main.Run()
	if code != 0 {
		return nil, fmt.Errorf("test exited with code %d", code)
	}
	return res, nil
}

func (app *App) execTest(t *testing.T, test Test) (*Result, error) {
	var cases []Case
	if len(test.Cases) == 0 {
		cases = []Case{Case{}}
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
	return res, nil
}

func (app *App) execTestCase(t Test, c Case) (*Result, error) {
	ctx := &hcl2.EvalContext{
		Functions: conf.Functions("."),
		Variables: map[string]cty.Value{
			"context": getContext(t.SourceLocator),
		},
	}

	vars, err := getVarialbles(ctx, t.Variables)
	if err != nil {
		return nil, err
	}

	ctx.Variables["var"] = vars

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

	res, err := app.execRun(nil, ctx, &t.Run, new(sync.Mutex))

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

func (app *App) execRunInternal(l *EventLogger, jobCtx *hcl2.EvalContext, run *RunJob) (*Result, error) {
	args := map[string]interface{}{}
	for k := range run.Args {
		var v cty.Value
		if diags := gohcl2.DecodeExpression(run.Args[k], jobCtx, &v); diags.HasErrors() {
			return nil, diags
		}
		vv, err := ctyToGo(v)
		if err != nil {
			return nil, err
		}
		args[k] = vv
	}

	if l != nil {
		if err := l.LogRun(run.Name, args); err != nil {
			return nil, err
		}
	}

	return app.run(l, run.Name, args, args)
}

func (app *App) execRun(l *EventLogger, jobCtx *hcl2.EvalContext, run *RunJob, m *sync.Mutex) (*Result, error) {
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
	jobCtx.Variables["run"] = runVal

	return res, err
}

func ctyToGo(v cty.Value) (interface{}, error) {
	var vv interface{}
	switch v.Type() {
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
	default:
		return nil, fmt.Errorf("handler for type %s not implemneted yet", v.Type().FriendlyName())
	}

	return vv, nil
}

func (app *App) execJobSteps(l *EventLogger, jobCtx *hcl2.EvalContext, results map[string]cty.Value, steps []Step, concurrency int) (*Result, error) {
	// TODO Sort steps by name and needs

	// TODO Clone this to avoid mutation
	hclCtx := jobCtx

	m := new(sync.Mutex)

	idToF := map[string]func() (*Result, error){}

	var dagNodeIds []string
	dagNodeIdToDeps := map[string][]string{}
	dagNodeIdToIndex := map[string]int{}

	var lastRes *Result
	for i := range steps {
		s := steps[i]

		f := func() (*Result, error) {
			var err error
			res, err := app.execRun(l, hclCtx, s.Run, m)
			if err != nil {
				return res, err
			}

			m.Lock()

			results[s.Name] = res.toCty()
			resultsCty := cty.ObjectVal(results)
			hclCtx.Variables["step"] = resultsCty

			m.Unlock()

			return res, nil
		}

		idToF[s.Name] = f

		dagNodeIdToIndex[s.Name] = i

		dagNodeIds = append(dagNodeIds, s.Name)

		if s.Needs != nil {
			dagNodeIdToDeps[s.Name] = *s.Needs
		} else {
			dagNodeIdToDeps[s.Name] = []string{}
		}
	}

	g := dag.New(dag.Nodes(dagNodeIds))
	for id, deps := range dagNodeIdToDeps {
		g.AddDependencies(id, deps)
	}

	plan, err := g.Plan()
	if err != nil {
		return nil, err
	}

	for _, nodes := range plan {
		ids := []string{}
		for _, n := range nodes {
			ids = append(ids, n.Id)
		}
		// Preserve the order of definitions
		sort.Slice(ids, func(i, j int) bool {
			return dagNodeIdToIndex[ids[i]] < dagNodeIdToIndex[ids[j]]
		})

		var wg sync.WaitGroup
		type result struct {
			r   *Result
			err error
		}
		rs := make([]result, len(ids))
		var rsm sync.Mutex

		workqueue := make(chan func())

		for c := 0; c < concurrency; c++ {
			go func() {
				for w := range workqueue {
					w()
				}
			}()
		}

		for i, _ := range ids {
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

func createJobContext(cc *HCL2Config, j JobSpec, givenParams map[string]interface{}, givenOpts map[string]interface{}) (*hcl2.EvalContext, error) {
	ctx := getContext(j.SourceLocator)

	params := map[string]cty.Value{}
	paramSpecs := append(append([]Parameter{}, cc.Parameters...), j.Parameters...)
	for _, p := range paramSpecs {
		v := givenParams[p.Name]

		var tpe cty.Type
		tpe, diags := typeexpr.TypeConstraint(p.Type)
		if diags != nil {
			return nil, diags
		}

		switch v.(type) {
		case nil:
			r := p.Default.Range()
			if r.Start != r.End {
				var vv cty.Value
				defCtx := &hcl2.EvalContext{
					Functions: conf.Functions("."),
					Variables: map[string]cty.Value{
						"context": ctx,
					},
				}
				if err := gohcl2.DecodeExpression(p.Default, defCtx, &vv); err != nil {
					return nil, err
				}
				if vv.Type() != tpe {
					return nil, errors.WithStack(fmt.Errorf("job %q: unexpected type of value %v provided to parameter %q: want %s, got %s", j.Name, vv, p.Name, tpe.FriendlyName(), vv.Type().FriendlyName()))
				}
				params[p.Name] = vv
				continue
			}
			return nil, fmt.Errorf("job %q: missing value for parameter %q", j.Name, p.Name)
		}

		if vty, err := gocty.ImpliedType(v); err != nil {
			return nil, err
		} else if vty != tpe {
			return nil, fmt.Errorf("job %q: unexpected type of option. want %q, got %q", j.Name, tpe.FriendlyNameForConstraint(), vty.FriendlyName())
		}

		val, err := gocty.ToCtyValue(v, tpe)
		if err != nil {
			return nil, err
		}
		params[p.Name] = val
	}

	opts := map[string]cty.Value{}
	optSpecs := append(append([]OptionSpec{}, cc.Options...), j.Options...)
	for _, op := range optSpecs {
		v := givenOpts[op.Name]

		var tpe cty.Type
		tpe, diags := typeexpr.TypeConstraint(op.Type)
		if diags != nil {
			return nil, diags
		}

		switch v.(type) {
		case nil:
			r := op.Default.Range()
			if r.Start != r.End {
				var vv cty.Value
				defCtx := &hcl2.EvalContext{
					Functions: conf.Functions("."),
					Variables: map[string]cty.Value{
						"context": ctx,
					},
				}
				if err := gohcl2.DecodeExpression(op.Default, defCtx, &vv); err != nil {
					return nil, err
				}
				if vv.Type() != tpe {
					return nil, errors.WithStack(fmt.Errorf("job %q: unexpected type of vaule %v provided to option %q: want %s, got %s", j.Name, vv, op.Name, tpe.FriendlyName(), vv.Type().FriendlyName()))
				}
				opts[op.Name] = vv
				continue
			}
			return nil, fmt.Errorf("job %q: missing value for option %q", j.Name, op.Name)
		}

		if vty, err := gocty.ImpliedType(v); err != nil {
			return nil, err
		} else if vty != tpe {
			return nil, fmt.Errorf("job %q: unexpected type of option. want %q, got %q", j.Name, tpe.FriendlyNameForConstraint(), vty.FriendlyName())
		}

		val, err := gocty.ToCtyValue(v, tpe)
		if err != nil {
			return nil, err
		}
		opts[op.Name] = val
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

	vars, err := getVarialbles(varCtx, varSpecs)
	if err != nil {
		return nil, err
	}

	jobCtx := &hcl2.EvalContext{
		Functions: conf.Functions("."),
		Variables: map[string]cty.Value{
			"param":   cty.ObjectVal(params),
			"opt":     cty.ObjectVal(opts),
			"var":     vars,
			"context": ctx,
		},
	}

	mod, err := getModule(jobCtx, cc.Module, j.Module)
	if err != nil {
		return nil, err
	}

	jobCtx.Variables["mod"] = mod

	return jobCtx, nil
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

				format = "yaml"
			case "job":
				var source SourceJob
				if err := gohcl2.DecodeBody(sourceSpec.Body, confCtx, &source); err != nil {
					return cty.NilVal, err
				}

				ctyArgs := map[string]cty.Value{}

				if err := gohcl2.DecodeExpression(source.Args, confCtx, &ctyArgs); err != nil {
					return cty.NilVal, err
				}

				args := map[string]interface{}{}
				for k, v := range ctyArgs {
					vv, err := ctyToGo(v)
					if err != nil {
						return cty.NilVal, err
					}
					args[k] = vv
				}

				res, err := app.Run(source.Name, args, args)
				if err != nil {
					return cty.NilVal, err
				}

				yamlData = []byte(res.Stdout)

				if source.Format != nil {
					format = *source.Format
				} else {
					format = "yaml"
				}
			default:
				return cty.DynamicVal, fmt.Errorf("config source %q is not implemented. It must be either \"file\" or \"job\", so that it looks like `source file {` or `source file {`", sourceSpec.Type)
			}

			m := map[string]interface{}{}

			switch format {
			case "yaml":
				if err := yaml.Unmarshal(yamlData, &m); err != nil {
					return cty.NilVal, err
				}
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

func getVarialbles(varCtx *hcl2.EvalContext, varSpecs []Variable) (cty.Value, error) {
	varFields := map[string]cty.Value{}
	for _, varSpec := range varSpecs {
		var tpe cty.Type

		if tv, _ := varSpec.Type.Value(nil); !tv.IsNull() {
			var diags hcl2.Diagnostics
			tpe, diags = typeexpr.TypeConstraint(varSpec.Type)
			if diags != nil {
				return cty.ObjectVal(varFields), diags
			}
		}

		if tpe.IsListType() && tpe.ListElementType().Equals(cty.String) {
			var v []string
			if err := gohcl2.DecodeExpression(varSpec.Value, varCtx, &v); err != nil {
				return cty.ObjectVal(varFields), err
			}
			if vty, err := gocty.ImpliedType(v); err != nil {
				return cty.ObjectVal(varFields), err
			} else if vty != tpe {
				return cty.ObjectVal(varFields), fmt.Errorf("unexpected type of option. want %q, got %q", tpe.FriendlyNameForConstraint(), vty.FriendlyName())
			}

			val, err := gocty.ToCtyValue(v, tpe)
			if err != nil {
				return cty.ObjectVal(varFields), err
			}
			varFields[varSpec.Name] = val
		} else {
			var v cty.Value

			if err := gohcl2.DecodeExpression(varSpec.Value, varCtx, &v); err != nil {
				return cty.ObjectVal(varFields), err
			}

			vty := v.Type()

			if !vty.Equals(tpe) {
				return cty.ObjectVal(varFields), fmt.Errorf("unexpected type of value for variable. want %q, got %q", tpe.FriendlyNameForConstraint(), vty.FriendlyName())
			}

			varFields[varSpec.Name] = v
		}
	}
	return cty.ObjectVal(varFields), nil
}

func getModule(ctx *hcl2.EvalContext, m1, m2 hcl2.Expression) (cty.Value, error) {
	var m hcl2.Expression

	if m2.Range().Start != m2.Range().End {
		m = m2
	} else if m1.Range().Start != m1.Range().End {
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
	return ex.Range().Start == ex.Range().End
}
