package app

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	hcl2 "github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
	hcl2parse "github.com/hashicorp/hcl/v2/hclparse"
	"github.com/mumoshu/hcl2test/pkg/conf"
	"github.com/pkg/errors"
	"github.com/variantdev/mod/pkg/shell"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

type hcl2Loader struct {
	Parser *hcl2parse.Parser
}

type configurable struct {
	Body hcl2.Body
}

func (l hcl2Loader) loadFile(filenames ...string) (*configurable, []*hcl2.File, error) {
	var files []*hcl2.File
	var diags hcl2.Diagnostics

	for _, filename := range filenames {
		var f *hcl2.File
		if strings.HasSuffix(filename, ".json") {
			f, diags = l.Parser.ParseJSONFile(filename)
		} else {
			f, diags = l.Parser.ParseHCLFile(filename)
		}
		files = append(files, f)
		if diags.HasErrors() {
			// Return diagnostics as an error; callers may type-assert this to
			// recover the original diagnostics, if it doesn't end up wrapped
			// in another error.
			return nil, files, diags
		}
	}

	body := hcl2.MergeFiles(files)

	return &configurable{
		Body: body,
	}, files, nil
}

type Config struct {
	Files       *[]string `hcl:"files,attr"`
	Contexts    *[]string `hcl:"contexts,attr"`
	Directories *[]string `hcl:"directories,attr"`
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

type ParameterSpec struct {
	Name string `hcl:"name,label"`

	Type    hcl2.Expression `hcl:"type,attr"`
	Default hcl2.Expression `hcl:"default,attr"`
	Envs    []EnvSource     `hcl:"env,block"`
	Jobs    []JobSource     `hcl:"job,block"`

	Description *string `hcl:"description,attr"`
}

type EnvSource struct {
	Name string `hcl:"name,label"`
}

type JobSource struct {
	Name      string                 `hcl:"name,label"`
	Arguments map[string]interface{} `hcl:",remain"`
}

type OptionSpec struct {
	Name string `hcl:"name,label"`

	Type        hcl2.Expression `hcl:"type,attr"`
	Default     hcl2.Expression `hcl:"default,attr"`
	Description *string         `hcl:"description,attr"`
	Short       *string         `hcl:"short,attr"`
}

type VariableSpec struct {
	Name string `hcl:"name,label"`

	Type  hcl2.Expression `hcl:"type,attr"`
	Value hcl2.Expression `hcl:"value,attr"`
}

type JobSpec struct {
	//Type string `hcl:"type,label"`
	Name string `hcl:"name,label"`

	Version *string `hcl:"version,attr"`

	Description *string         `hcl:"description,attr"`
	Parameters  []ParameterSpec `hcl:"parameter,block"`
	Options     []OptionSpec    `hcl:"option,block"`
	Variables   []VariableSpec  `hcl:"variable,block"`

	Concurrency *int `hcl:"concurrency,attr"`

	SourceLocator hcl2.Expression `hcl:"__source_locator,attr"`

	Exec   *Exec           `hcl:"exec,block"`
	Assert []Assert        `hcl:"assert,block"`
	Fail   hcl2.Expression `hcl:"fail,attr"`
	Run    *RunJob         `hcl:"run,block"`

	Steps []Step `hcl:"step,block"'`
}

type Assert struct {
	Name string `hcl:"name,label"`

	Condition hcl2.Expression `hcl:"condition,attr"`
}

type HCL2Config struct {
	Config  *Config   `hcl:"config,block"`
	Jobs    []JobSpec `hcl:"job,block"`
	Tests   []Test    `hcl:"test,block"`
	JobSpec `hcl:",remain"`
}

type Test struct {
	Name string `hcl:"name,label"`

	Cases  []Case   `hcl:"case,block"`
	Run    RunJob   `hcl:"run,block"`
	Assert []Assert `hcl:"assert,block"`
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
	Files     map[string]*hcl2.File
	Config    *HCL2Config
	jobByName map[string]JobSpec

	Stdout, Stderr io.Writer

	TraceCommands []string
}

func New(dir string) (*App, error) {
	l := &hcl2Loader{
		Parser: hcl2parse.NewParser(),
	}

	files, err := conf.FindHCLFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to get .hcl files: %v", err)
	}

	//file := "complex.hcl"
	c, hclFiles, err := l.loadFile(files...)
	nameToFiles := map[string]*hcl2.File{}
	for i := range files {
		nameToFiles[files[i]] = hclFiles[i]
	}

	app := &App{
		Files: nameToFiles,
	}
	if err != nil {
		return app, err
	}

	cc, err := c.HCL2Config()
	if err != nil {
		return app, err
	}

	jobByName := map[string]JobSpec{}
	for _, j := range cc.Jobs {
		jobByName[j.Name] = j
	}
	jobByName[""] = cc.JobSpec

	app.Config = cc
	app.jobByName = jobByName

	return app, nil
}

func (app *App) Run(cmd string, args map[string]interface{}, opts map[string]interface{}) (*Result, error) {
	jobByName := app.jobByName
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

	needs := map[string]cty.Value{}
	res, err := app.execJobSteps(jobCtx, needs, j.Steps)
	if res != nil || err != nil {
		return res, err
	}

	return app.execJob(j, jobCtx)
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

func (app *App) execJob(j JobSpec, ctx *hcl2.EvalContext) (*Result, error) {
	var res *Result
	var err error

	if j.Exec != nil {
		var cmd string
		if diags := gohcl2.DecodeExpression(j.Exec.Command, ctx, &cmd); diags.HasErrors() {
			return nil, diags
		}

		var args []string
		if diags := gohcl2.DecodeExpression(j.Exec.Args, ctx, &args); diags.HasErrors() {
			return nil, diags
		}

		var env map[string]string
		if diags := gohcl2.DecodeExpression(j.Exec.Env, ctx, &env); diags.HasErrors() {
			return nil, diags
		}

		res, err = app.execCmd(cmd, args, env, true)
	} else if j.Run != nil {
		res, err = app.execRun(ctx, j.Run)
	}

	if j.Assert != nil && len(j.Assert) > 0 {
		for _, a := range j.Assert {
			if err := app.execAssert(ctx, a); err != nil {
				return nil, err
			}
		}
		return &Result{}, nil
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
				v, err := app.ctyToGo(ctyValue)
				if err != nil {
					panic(err)
				}
				src := strings.TrimSpace(string(b[t.SourceRange().Start.Byte : t.SourceRange().End.Byte]))
				vars = append(vars, fmt.Sprintf("%s=%v (%T)", string(src), v, v))
			}
		}

		return fmt.Errorf("assertion %q failed: this expression must be true, but was false: %s, where %s", a.Name, expr, strings.Join(vars, " "))
	}

	return nil
}

func (app *App) RunTests() (*Result, error) {
	ctx := &hcl2.EvalContext{
		Functions: conf.Functions("."),
		Variables: map[string]cty.Value{},
	}

	var res *Result
	var err error
	for _, t := range app.Config.Tests {
		res, err = app.execTest(t, ctx)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (app *App) execTest(t Test, ctx *hcl2.EvalContext) (*Result, error) {
	if len(t.Cases) > 0 {
		var res *Result
		var err error
		for _, c := range t.Cases {
			res, err = app.execTestCase(t, c, ctx)
			if err != nil {
				return nil, err
			}
		}
		return res, nil
	}
	return app.execTestCase(t, Case{}, ctx)
}

func (app *App) execTestCase(t Test, c Case, ctx *hcl2.EvalContext) (*Result, error) {
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

	res, err := app.execRun(ctx, &t.Run)

	// If there are one ore more assert(s), do not fail immediately and let the assertion(s) decide
	if t.Assert != nil && len(t.Assert) > 0 {
		for _, a := range t.Assert {
			if err := app.execAssert(ctx, a); err != nil {
				return nil, err
			}
		}
		return &Result{}, nil
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
	return cty.ObjectVal(map[string]cty.Value{
		"stdout":     cty.StringVal(res.Stdout),
		"stderr":     cty.StringVal(res.Stderr),
		"exitstatus": cty.NumberIntVal(int64(res.ExitStatus)),
	})
}

func (app *App) execRun(jobCtx *hcl2.EvalContext, run *RunJob) (*Result, error) {
	args := map[string]interface{}{}
	for k := range run.Args {
		var v cty.Value
		if diags := gohcl2.DecodeExpression(run.Args[k], jobCtx, &v); diags.HasErrors() {
			return &Result{
				Noop: false,
			}, diags
		}
		vv, err := app.ctyToGo(v)
		if err != nil {
			return nil, err
		}
		args[k] = vv
	}

	var err error
	res, err := app.Run(run.Name, args, args)

	runFields := map[string]cty.Value{}
	if res != nil {
		runFields["res"] = res.toCty()
	}
	if err != nil {
		runFields["err"] = cty.StringVal(err.Error())
	} else {
		runFields["err"] = cty.StringVal("")
	}
	runVal := cty.ObjectVal(runFields)
	jobCtx.Variables["run"] = runVal

	return res, err
}

func (app *App) ctyToGo(v cty.Value) (interface{}, error) {
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
	default:
		return nil, fmt.Errorf("handler for type %s not implemneted yet", v.Type().FriendlyName())
	}

	return vv, nil
}

func (app *App) execJobSteps(jobCtx *hcl2.EvalContext, stepResults map[string]cty.Value, steps []Step) (*Result, error) {
	// TODO Sort steps by name and needs

	// TODO Clone this to avoid mutation
	stepCtx := jobCtx

	var lastRes *Result
	for _, s := range steps {
		var err error
		lastRes, err = app.execRun(stepCtx, s.Run)
		if err != nil {
			return lastRes, err
		}
		stepResults[s.Name] = lastRes.toCty()
		stepResultsVal := cty.ObjectVal(stepResults)
		stepCtx.Variables["step"] = stepResultsVal
	}
	return lastRes, nil
}

func createJobContext(cc *HCL2Config, j JobSpec, givenParams map[string]interface{}, givenOpts map[string]interface{}) (*hcl2.EvalContext, error) {
	params := map[string]cty.Value{}
	paramSpecs := append(append([]ParameterSpec{}, cc.Parameters...), j.Parameters...)
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
				}
				if err := gohcl2.DecodeExpression(p.Default, defCtx, &vv); err != nil {
					return nil, err
				}
				if vv.Type() != tpe {
					return nil, errors.WithStack(fmt.Errorf("unexpected type of value %v provided to parameter %q: want %s, got %s", vv, p.Name, tpe.FriendlyName(), vv.Type().FriendlyName()))
				}
				params[p.Name] = vv
				continue
			}
			return nil, fmt.Errorf("missing value for parameter %q", p.Name)
		}

		if vty, err := gocty.ImpliedType(v); err != nil {
			return nil, err
		} else if vty != tpe {
			return nil, fmt.Errorf("unexpected type of option. want %q, got %q", tpe.FriendlyNameForConstraint(), vty.FriendlyName())
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
				}
				if err := gohcl2.DecodeExpression(op.Default, defCtx, &vv); err != nil {
					return nil, err
				}
				if vv.Type() != tpe {
					return nil, errors.WithStack(fmt.Errorf("unexpected type of vaule %v provided to option %q: want %s, got %s", vv, op.Name, tpe.FriendlyName(), vv.Type().FriendlyName()))
				}
				opts[op.Name] = vv
				continue
			}
			return nil, fmt.Errorf("missing value for option %q", op.Name)
		}

		if vty, err := gocty.ImpliedType(v); err != nil {
			return nil, err
		} else if vty != tpe {
			return nil, fmt.Errorf("unexpected type of option. want %q, got %q", tpe.FriendlyNameForConstraint(), vty.FriendlyName())
		}

		val, err := gocty.ToCtyValue(v, tpe)
		if err != nil {
			return nil, err
		}
		opts[op.Name] = val
	}

	context := map[string]cty.Value{}
	{
		context["sourcedir"] = cty.StringVal(filepath.Dir(j.SourceLocator.Range().Filename))
	}

	vars := map[string]cty.Value{}
	varSpecs := append(append([]VariableSpec{}, cc.Variables...), j.Variables...)
	varCtx := &hcl2.EvalContext{
		Functions: conf.Functions("."),
		Variables: map[string]cty.Value{
			"param":   cty.ObjectVal(params),
			"opt":     cty.ObjectVal(opts),
			"context": cty.ObjectVal(context),
		},
	}
	for _, varSpec := range varSpecs {
		var tpe cty.Type

		if tv, _ := varSpec.Type.Value(nil); !tv.IsNull() {
			var diags hcl2.Diagnostics
			tpe, diags = typeexpr.TypeConstraint(varSpec.Type)
			if diags != nil {
				return nil, diags
			}
		}

		var v cty.Value

		if tpe.IsListType() {
			v = cty.ListValEmpty(*tpe.ListElementType())
		} else if tpe.IsMapType() {
			v = cty.MapValEmpty(*tpe.MapElementType())
		}

		if err := gohcl2.DecodeExpression(varSpec.Value, varCtx, &v); err != nil {
			return nil, err
		}

		vars[varSpec.Name] = v
	}

	ctx := &hcl2.EvalContext{
		Functions: conf.Functions("."),
		Variables: map[string]cty.Value{
			"param":   cty.ObjectVal(params),
			"opt":     cty.ObjectVal(opts),
			"var":     cty.ObjectVal(vars),
			"context": cty.ObjectVal(context),
		},
	}

	return ctx, nil
}
