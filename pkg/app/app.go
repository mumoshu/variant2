package app

import (
	"fmt"
	"os"
	"strings"

	hcl2 "github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/dynblock"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
	hcl2parse "github.com/hashicorp/hcl/v2/hclparse"
	"github.com/mumoshu/hcl2test/pkg/conf"
	"github.com/pkg/errors"
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

type Runner struct {
}

type Step struct {
	Name *string `hcl:"name,attr"`

	Needs *[]string `hcl:"need,attr"`

	DeferredStep hcl2.Body `hcl:",remain"`
}

type DeferredStep struct {
	Runner *Runner `hcl:"runner,block"`

	ScriptToRun hcl2.Expression `hcl:"script,attr"`

	AssertionToRun hcl2.Expression `hcl:"assert,attr"`

	Fail hcl2.Expression `hcl:"fail,attr"`

	RunJob *JobToRun `hcl:"job,block"`
}

type JobToRun struct {
	Name string `hcl:"name,label"`

	Arguments map[string]cty.Value `hcl:",remain"`
}

type DeferredRuns struct {
	Steps []Step `hcl:"step,block"'`

	Runs []Run `hcl:"run,block"`
}

type Run struct {
	Concurrency *int `hcl:"concurrency,attr"`

	Steps []Step `hcl:"step,block"'`

	Phases []Phase `hcl:"phase,block"`
}

type Phase struct {
	Name *string `hcl:"name,attr"`

	Needs *[]string `hcl:"needs,attr"`

	DeferredRuns hcl2.Body `hcl:",remain"`
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

	DeferredRuns hcl2.Body `hcl:",remain"`
}

type HCL2Config struct {
	Config  *Config    `hcl:"config,block"`
	Jobs    []JobSpec `hcl:"job,block"`
	Tests   []Test    `hcl:"test,block"`
	JobSpec `hcl:",remain"`
}

type Test struct {
	Body hcl2.Body `hcl:",remain"`
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

func (app *App) Run(cmd string, args map[string]interface{}, opts map[string]interface{}) (*DeferredRuns, error) {
	jobByName := app.jobByName
	cc := app.Config

	j, ok := jobByName[cmd]
	if !ok {
		j, ok = jobByName[""]
		if !ok {
			panic(fmt.Errorf("command %q not found", cmd))
		}
	}
	jobCtx, err := createJobRunContext(cc, j, args, opts)
	if err != nil {
		app.PrintError(err)
		return nil, err
	}
	//
	//if len(j.Steps) > 0 {
	//	needs := map[string]cty.Value{}
	//
	//	stepCtx := jobCtx
	//
	//	if err := runSteps(stepCtx, needs, j.Steps); err != nil {
	//		app.ExitWithError(err)
	//	}
	//
	//	continue
	//}

	runs, err := createRuns(jobCtx, j.DeferredRuns)
	if err != nil {
		return runs, err
	}

	if len(runs.Steps) > 0 {
		needs := map[string]cty.Value{}

		stepCtx := jobCtx

		if err := app.runSteps(stepCtx, needs, runs.Steps); err != nil {
			app.ExitWithError(err)
		}

		return runs, nil
	}

	phaseResults := map[string]cty.Value{}

	stepCtx := jobCtx

	for _, run := range runs.Runs {
		if len(run.Steps) > 0 {
			if run.Concurrency == nil {
				run.Concurrency = j.Concurrency
			}
			needs := map[string]cty.Value{}

			if err := app.runSteps(stepCtx, needs, run.Steps); err != nil {
				return runs, err
			}

			continue
		}

		// TODO Sort phases by name and needs

		for _, phase := range run.Phases {
			phasedRuns, err := createRuns(stepCtx, phase.DeferredRuns)
			if err != nil {
				return runs, err
			}

			for _, pRun := range phasedRuns.Runs {
				needs := map[string]cty.Value{}

				if err := app.runSteps(stepCtx, needs, pRun.Steps); err != nil {
					return runs, err
				}
			}

			if phase.Name != nil && *phase.Name != "" {
				phaseResults[*phase.Name] = cty.ObjectVal(stepCtx.Variables)
				phaseResultsVal := cty.ObjectVal(phaseResults)
				stepCtx.Variables["phase"] = phaseResultsVal
			}
		}
	}

	return runs, nil
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

func (app *App) runScript(script string) (cty.Value, error) {
	println(script)

	// TODO exec with the runner
	stdout := script
	stderr := script

	return cty.ObjectVal(map[string]cty.Value{
		"stdout": cty.StringVal(stdout),
		"stderr": cty.StringVal(stderr),
	}), nil
}

func (app *App) runSteps(stepCtx *hcl2.EvalContext, stepResults map[string]cty.Value, steps []Step) error {
	// TODO Sort steps by name and needs

	for _, s := range steps {
		var def DeferredStep
		body := dynblock.Expand(s.DeferredStep, stepCtx)
		if err := gohcl2.DecodeBody(body, stepCtx, &def); err != nil {
			return err
		}
		if def.AssertionToRun != nil {
			r := def.AssertionToRun.Range()
			if r.Start != r.End {
				var assert bool

				diags := gohcl2.DecodeExpression(def.ScriptToRun, stepCtx, &assert)
				if diags.HasErrors() {
					return diags
				}

				if !assert {
					return fmt.Errorf("assertion failed: %s", def.ScriptToRun)
				}
			}
		}
		if def.ScriptToRun != nil {
			var script string

			diags := gohcl2.DecodeExpression(def.ScriptToRun, stepCtx, &script)
			if diags.HasErrors() {
				return diags
			}

			res, err := app.runScript(script)
			if err != nil {
				return err
			}

			// Save step outputs to allow reuses by later steps
			if s.Name != nil {
				stepResults[*s.Name] = res
				stepResultsVal := cty.ObjectVal(stepResults)
				stepCtx.Variables["step"] = stepResultsVal
			}
		} else if def.RunJob != nil {
			panic("not implemented")
		} else {
			panic("either script or job must be defined")
		}
	}
	return nil
}

func createJobRunContext(cc *HCL2Config, j JobSpec, givenParams map[string]interface{}, givenOpts map[string]interface{}) (*hcl2.EvalContext, error) {
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

	vars := map[string]cty.Value{}
	varSpecs := append(append([]VariableSpec{}, cc.Variables...), j.Variables...)
	varCtx := &hcl2.EvalContext{
		Functions: conf.Functions("."),
		Variables: map[string]cty.Value{
			"param": cty.ObjectVal(params),
			"opt":   cty.ObjectVal(opts),
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
			"param": cty.ObjectVal(params),
			"opt":   cty.ObjectVal(opts),
			"var":   cty.ObjectVal(vars),
		},
	}

	return ctx, nil
}

func createRuns(ctx *hcl2.EvalContext, body hcl2.Body) (*DeferredRuns, error) {
	deferredRuns := &DeferredRuns{}

	expandedBody := dynblock.Expand(body, ctx)

	diags := gohcl2.DecodeBody(expandedBody, ctx, deferredRuns)
	if diags.HasErrors() {
		// We return the diags as an implementation of error, which the
		// caller than then type-assert if desired to recover the individual
		// diagnostics.
		// FIXME: The current API gives us no way to return warnings in the
		// absence of any errors.
		return deferredRuns, diags
	}

	return deferredRuns, nil
}
