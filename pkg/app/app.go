package app

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	multierror "github.com/hashicorp/go-multierror"
	hcl2 "github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	gohcl2 "github.com/hashicorp/hcl/v2/gohcl"
	"github.com/imdario/mergo"
	"github.com/kr/text"
	"github.com/pkg/errors"
	"github.com/variantdev/dag/pkg/dag"
	"github.com/variantdev/mod/pkg/shell"
	"github.com/variantdev/mod/pkg/variantmod"
	"github.com/variantdev/vals"
	ctyyaml "github.com/zclconf/go-cty-yaml"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/gocty"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/mumoshu/variant2/pkg/conf"
)

const (
	NoRunMessage = "nothing to run"

	FormatYAML = "yaml"

	FormatText = "text"
)

func (app *App) Run(cmd string, args map[string]interface{}, opts map[string]interface{}, fs ...SetOptsFunc) (*Result, error) {
	var f SetOptsFunc
	if len(fs) > 0 {
		f = fs[0]
	}

	jr, err := app.Job(nil, nil, cmd, args, opts, f, true)
	if err != nil {
		return nil, err
	}

	res, err := jr()
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (app *App) run(jobCtx *JobContext, l *EventLogger, cmd string, args map[string]interface{}, streamOutput bool) (*Result, error) {
	if l != nil {
		if err := l.LogRun(cmd, args); err != nil {
			return nil, err
		}
	}

	jr, err := app.Job(jobCtx, l, cmd, args, args, nil, streamOutput)
	if err != nil {
		if cmd != "" {
			return nil, xerrors.Errorf("job %q: %w", cmd, err)
		}

		return nil, err
	}

	res, err := jr()
	if err != nil {
		if cmd != "" {
			return nil, xerrors.Errorf("job %q: %w", cmd, err)
		}

		return nil, err
	}

	return res, nil
}

func (app *App) Job(jobCtx *JobContext, l *EventLogger, cmd string, args map[string]interface{}, opts map[string]interface{}, f SetOptsFunc, streamOutput bool) (func() (*Result, error), error) {
	jobByName := app.JobByName

	j, cmdDefined := jobByName[cmd]
	if !cmdDefined {
		var ok bool

		j, ok = jobByName[""]
		if !ok {
			return nil, fmt.Errorf("command %q not found", cmd)
		}
	}

	return func() (*Result, error) {
		cc := app.Config

		// execMatcher is the only object that is inherited from the parent to the child jobContext
		var execMatcher *execMatcher

		if jobCtx != nil {
			execMatcher = jobCtx.execMatcher
		}

		jobCtx, err := app.createJobContext(cc, j, args, opts, f)
		if err != nil {
			app.PrintError(err)

			return nil, err
		}

		jobCtx.execMatcher = execMatcher

		jobEvalCtx := jobCtx.evalContext

		if l == nil {
			l = NewEventLogger(cmd, args, opts)
			l.Stderr = app.Stderr

			if app.Trace != "" {
				l.Register(app.newTracingLogCollector())
			}
		}

		//nolint:nestif
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

		var depStdout string

		var lastDepRes *Result

		{
			if j.Deps != nil {
				for i := range j.Deps {
					d := j.Deps[i]

					var err error

					lastDepRes, err = app.execMultiRun(l, jobCtx, &d, streamOutput)
					if err != nil {
						return nil, err
					}

					depStdout += lastDepRes.Stdout
				}
			}
		}

		if err := app.checkoutSources(l, jobCtx, j.Sources, concurrency); err != nil {
			return nil, err
		}

		r, err := app.execJobSteps(l, jobCtx, needs, j.Steps, concurrency, streamOutput)
		if err != nil {
			app.PrintDiags(err)

			return r, err
		}

		if r == nil {
			jobRes, err := app.execJob(l, j, jobCtx, streamOutput)
			if err != nil {
				app.PrintDiags(err)

				return jobRes, err
			}

			r = jobRes
		}

		if r == nil {
			// The job contained only `depends_on` block(s)
			// Treat the result of depends_on as the result of this job
			if lastDepRes != nil {
				lastDepRes.Stdout = depStdout

				return lastDepRes, nil
			}

			if err == nil && !cmdDefined {
				return nil, xerrors.Errorf("job %q is not defined", cmd)
			} else if err == nil {
				return nil, errors.New(NoRunMessage)
			}
		} else {
			// The job contained job or step(s).
			// If we also had depends_on block(s), concat all the results
			r.Stdout = depStdout + r.Stdout
		}

		app.PrintDiags(err)

		return r, err
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
	diags := hcl2.Diagnostics{}
	if errors.As(err, &diags) {
		app.WriteDiags(diags)
	} else {
		fmt.Fprintf(os.Stderr, "%v", err)
	}
}

func (app *App) PrintDiags(err error) {
	diags := hcl2.Diagnostics{}
	if errors.As(err, &diags) {
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

func (app *App) execCmd(ctx *JobContext, cmd Command, log bool) (*Result, error) {
	var execM *execMatcher

	if ctx != nil {
		execM = ctx.execMatcher
	}

	if execM == nil {
		execM = &execMatcher{}
	}

	// If we have one ore more pending exec expectations, never run the actual command.
	// Instead, do validate the execCmd run against the expectation.
	if len(execM.expectedExecs) > 0 {
		execM.execInvocationCount++

		expectation := execM.expectedExecs[0]

		if cmd.Name != expectation.Command {
			return nil, fmt.Errorf("unexpected exec %d: expected command %q, got %q", execM.execInvocationCount, expectation.Command, cmd.Name)
		}

		if diff := cmp.Diff(expectation.Args, cmd.Args); diff != "" {
			return nil, fmt.Errorf("unexpected exec %d: expected args %v, got %v", execM.execInvocationCount, expectation.Args, cmd.Args)
		}

		if diff := cmp.Diff(expectation.Dir, cmd.Dir); diff != "" {
			return nil, fmt.Errorf("unexpected exec %d: expected dir %q, got %q", execM.execInvocationCount, expectation.Dir, cmd.Dir)
		}

		// Pop the successful command expectation so that on next execCmd call, we can
		// use expectedExecs[0] as the next expectation to be checked.
		execM.expectedExecs = execM.expectedExecs[1:]

		return &Result{Validated: true}, nil
	} else if execM.execInvocationCount > 0 {
		return nil, fmt.Errorf("unexpected exec %d: fix the test by adding an expect block for this exec, or fix the test target: %v", execM.execInvocationCount+1, cmd)
	}

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

func (app *App) execJob(l *EventLogger, j JobSpec, jobCtx *JobContext, streamOutput bool) (*Result, error) {
	var res *Result

	var err error

	var cmd string

	var args []string

	var env map[string]string

	var dir string

	evalCtx := jobCtx.evalContext

	//nolint:nestif
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

		res, err = app.execCmd(jobCtx, c, streamOutput)
		if err := l.LogExec(cmd, args); err != nil {
			return nil, err
		}
	} else {
		var jobExists bool

		res, jobExists, err = app.runJobInBody(l, jobCtx, j.Body, streamOutput)

		if err != nil {
			return nil, err
		}

		if !jobExists && j.Assert != nil {
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

	return res, err
}

func (app *App) execAssert(ctx *hcl2.EvalContext, a Assert) error {
	var assert bool

	cond := a.Condition

	diags := gohcl2.DecodeExpression(cond, ctx, &assert)
	if diags.HasErrors() {
		return diags
	}

	//nolint:nestif
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

				vars = append(vars, fmt.Sprintf("%s (%T) =\n%v", src, v, v))
			}
		}

		msg := fmt.Sprintf(`

EXPRESSION:

%s

VARIABLES:

%s
`, strings.TrimSpace(string(expr)), strings.Join(vars, "\n"))

		retErr := fmt.Errorf(`assertion %q failed: this expression must be true, but was false%s`, a.Name, msg)

		return retErr
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
				t.Fatalf("%s: %v", c.SourceLocator.Range(), err)
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

	ctx, err := setVariables(ctx, t.Variables)
	if err != nil {
		return nil, err
	}

	caseFields := map[string]cty.Value{}

	caseFieldsDAG := dag.New()

	caseFieldToExpr := map[string]hcl2.Expression{}

	for fieldName, expr := range c.Args {
		deps := map[string]struct{}{}

		for _, v := range expr.Variables() {
			if !v.IsRelative() && v.RootName() == "case" {
				// For `fieldName` "bar", `v.RootName()` is "case" which corresponds `case` of `case.foo` in
				// case "ok" {
				//    foo = "FOO"
				//    bar = case.foo
				// }
				//
				// `v.SimpleSplit().Rel[0]` is the TraverseAttr of `foo` in `case.foo`.
				if r, ok := v.SimpleSplit().Rel[0].(hcl2.TraverseAttr); ok {
					// r.Name is "foo" that corresponds `foo` in `case.foo`.
					deps[r.Name] = struct{}{}
				}
			}
		}

		var depFieldNames []string

		for n := range deps {
			depFieldNames = append(depFieldNames, n)
		}

		caseFieldsDAG.Add(fieldName, dag.Dependencies(depFieldNames))

		caseFieldToExpr[fieldName] = expr
	}

	caseFieldsTopology, err := caseFieldsDAG.Sort()
	if err != nil {
		return nil, xerrors.Errorf("sorting DAG of dependencies: %w", err)
	}

	var sortedCaseFieldNames []string

	for _, p := range caseFieldsTopology {
		for _, pi := range p {
			sortedCaseFieldNames = append(sortedCaseFieldNames, pi.Id)
		}
	}

	for _, k := range sortedCaseFieldNames {
		expr, ok := caseFieldToExpr[k]
		if !ok {
			return nil, fmt.Errorf("BUG: No expression found for case field %q", k)
		}

		var v cty.Value
		if diags := gohcl2.DecodeExpression(expr, ctx, &v); diags.HasErrors() {
			return nil, diags
		}

		caseFields[k] = v

		caseVal := cty.ObjectVal(caseFields)
		ctx.Variables["case"] = caseVal
	}

	jobCtx := &JobContext{
		evalContext: ctx,
		globalArgs:  map[string]interface{}{},
		execMatcher: &execMatcher{},
	}

	expectedExecs := []expectedExec{}

	for _, e := range t.ExpectedExecs {
		var cmd string

		if diags := gohcl2.DecodeExpression(e.Command, ctx, &cmd); diags.HasErrors() {
			return nil, diags
		}

		var args []string

		if diags := gohcl2.DecodeExpression(e.Args, ctx, &args); diags.HasErrors() {
			return nil, diags
		}

		var dir string

		if !IsExpressionEmpty(e.Dir) {
			if diags := gohcl2.DecodeExpression(e.Dir, ctx, &dir); diags.HasErrors() {
				return nil, diags
			}
		}

		expectedExecs = append(expectedExecs, expectedExec{
			Command: cmd,
			Args:    args,
			Dir:     dir,
		})
	}

	jobCtx.execMatcher.expectedExecs = expectedExecs

	res, err := app.runJobAndUpdateContext(nil, jobCtx, eitherJobRun{static: &t.Run}, new(sync.Mutex), true)

	if res == nil && err != nil {
		return nil, err
	}

	// If there are one ore more assert(s), do not fail immediately and let the assertion(s) decide
	if t.Assert != nil && len(t.Assert) > 0 {
		var lines []string

		for _, a := range t.Assert {
			if err := app.execAssert(jobCtx.evalContext, a); err != nil {
				if strings.HasPrefix(err.Error(), "assertion \"") {
					return nil, fmt.Errorf("case %q: %w", c.Name, err)
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
	Stdout    string
	Stderr    string
	Undefined bool
	Skipped   bool
	// Cancelled is set to true when and only when the original command execution has been scheduled concurrently,
	// but was cancelled before it was actually executed.
	Cancelled bool

	// Validated is set to true when and only when the command execution was successfully validated against the mock
	Validated bool

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

func (app *App) dispatchRunJob(l *EventLogger, jobCtx *JobContext, run eitherJobRun, streamOutput bool) (*Result, error) {
	var jobRun *jobRun

	var err error

	//nolint:nestif
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

		if jobRun.Skipped {
			if err := l.append(Event{
				Type: "run:skipped",
				Time: time.Now(),
				Run: &RunEvent{
					Job:  jobRun.Name,
					Args: map[string]interface{}{},
				},
			}); err != nil {
				return nil, err
			}

			return &Result{
				Skipped: true,
			}, nil
		}
	}

	return app.run(jobCtx, l, jobRun.Name, jobRun.Args, streamOutput)
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

func (app *App) execMultiRun(l *EventLogger, jobCtx *JobContext, r *DependsOn, streamOutput bool) (*Result, error) {
	ctyItems := []cty.Value{}

	items := []interface{}{}

	if !IsExpressionEmpty(r.Items) {
		if err := gohcl2.DecodeExpression(r.Items, jobCtx.evalContext, &ctyItems); err != nil {
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
			v, err := goToCty(item)
			if err != nil {
				return nil, err
			}

			args, err := buildArgsFromExpr(jobCtx.WithVariable("item", v).Ptr(), r.Args)
			if err != nil {
				return nil, err
			}

			res, err := app.run(jobCtx, l, r.Name, args, streamOutput)
			if err != nil {
				return res, err
			}

			stdout += res.Stdout + "\n"
		}

		return &Result{
			Stdout:     stdout,
			Stderr:     "",
			Undefined:  false,
			ExitStatus: 0,
		}, nil
	}

	args, err := buildArgsFromExpr(jobCtx, r.Args)
	if err != nil {
		return nil, err
	}

	res, err := app.run(jobCtx, l, r.Name, args, streamOutput)
	if err != nil {
		return res, err
	}

	res.Stdout += "\n"

	return res, nil
}

func (app *App) runJobAndUpdateContext(l *EventLogger, jobCtx *JobContext, run eitherJobRun, m sync.Locker, streamOutput bool) (*Result, error) {
	res, err := app.dispatchRunJob(l, jobCtx, run, streamOutput)

	if res == nil {
		res = &Result{ExitStatus: 1, Stderr: err.Error()}
	} else if res.Cancelled {
		res = &Result{ExitStatus: 3, Stderr: err.Error()}
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

func (app *App) execJobSteps(l *EventLogger, jobCtx *JobContext, results map[string]cty.Value, steps []Step, concurrency int, streamOutput bool) (*Result, error) {
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
			res, err := app.runJobAndUpdateContext(l, &stepCtx, eitherJobRun{static: &s.Run}, m, streamOutput)
			if err != nil {
				return res, xerrors.Errorf("step %q: %w", s.Name, err)
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
		return nil, xerrors.Errorf("calculating DAG of dependencies: %w", err)
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

		var cancelled bool

		for i := range ids {
			id := ids[i]

			f := idToF[id]

			ii := i

			wg.Add(1)
			workqueue <- func() {
				defer wg.Done()

				rsm.Lock()
				if cancelled {
					rs[ii] = result{r: &Result{Cancelled: true}}
					rsm.Unlock()

					return
				}
				rsm.Unlock()

				r, err := f()

				rsm.Lock()
				defer rsm.Unlock()
				rs[ii] = result{r: r, err: err}
				if err != nil {
					cancelled = true
				}
			}
		}

		wg.Wait()

		lastRes = rs[len(rs)-1].r

		var rese *multierror.Error

		for i := range rs {
			if e := rs[i].err; e != nil {
				rese = multierror.Append(rese, e)
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

	//nolint:nestif
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
		//nolint:wrapcheck
		return nil, nil, err
	}

	return &val, &tpe, nil
}

type JobContext struct {
	evalContext *hcl2.EvalContext

	globalArgs map[string]interface{}

	execMatcher *execMatcher
}

type execMatcher struct {
	execInvocationCount int
	expectedExecs       []expectedExec
}

func (c *JobContext) WithEvalContext(evalCtx *hcl2.EvalContext) JobContext {
	return JobContext{
		evalContext: evalCtx,
		globalArgs:  c.globalArgs,
	}
}

func (c *JobContext) WithVariable(name string, v cty.Value) JobContext {
	newEvalCtx := cloneEvalContext(c.evalContext)

	newEvalCtx.Variables[name] = v

	return c.WithEvalContext(newEvalCtx)
}

func (c JobContext) Ptr() *JobContext {
	return &c
}

func (app *App) createJobContext(cc *HCL2Config, j JobSpec, givenParams map[string]interface{}, givenOpts map[string]interface{}, f SetOptsFunc) (*JobContext, error) {
	ctx := getContext(j.SourceLocator)

	globalParams, err := setParameterValues("global parameter", ctx, cc.Parameters, givenParams)
	if err != nil {
		return nil, err
	}

	localParams, err := setParameterValues("parameter", ctx, j.Parameters, givenParams)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	localOpts, err := setOptionValues("option", ctx, j.Options, givenOpts, f)
	if err != nil {
		return nil, err
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

	modCtx := &hcl2.EvalContext{
		Functions: conf.Functions("."),
		Variables: map[string]cty.Value{
			"param":   cty.ObjectVal(params),
			"opt":     cty.ObjectVal(opts),
			"context": ctx,
		},
	}

	mod, err := getModule(modCtx, cc.Module, j.Module)
	if err != nil {
		return nil, err
	}

	confEvalCtx := cloneEvalContext(modCtx)
	confEvalCtx.Variables["mod"] = mod

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

	confJobCtx := &JobContext{
		evalContext: confEvalCtx,
		globalArgs:  globalArgs,
	}

	varSpecs := append(append([]Variable{}, cc.Variables...), j.Variables...)

	getConfigs := func(j JobSpec) []Config { return j.Configs }
	configs := append(append([]Config{}, getConfigs(cc.JobSpec)...), getConfigs(j)...)

	getSecrets := func(j JobSpec) []Config { return j.Secrets }
	secrets := append(append([]Config{}, getSecrets(cc.JobSpec)...), getSecrets(j)...)

	secretRefsEvaluator, err := vals.New(vals.Options{CacheSize: 100})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize vals: %w", err)
	}

	g := func(m map[string]interface{}) (map[string]interface{}, error) {
		return secretRefsEvaluator.Eval(m)
	}

	updatedContext, err := app.addConfigsAndVariables(confJobCtx, varSpecs, configs, secrets, g)
	if err != nil {
		return nil, err
	}

	jobCtx := confJobCtx.WithEvalContext(updatedContext).Ptr()

	return jobCtx, nil
}

//nolint:unused
func (app *App) getConfigs(jobCtx *JobContext, confType string, confSpecs []Config, g func(map[string]interface{}) (map[string]interface{}, error)) (cty.Value, error) {
	confCtx := jobCtx.evalContext

	confFields := map[string]cty.Value{}

	for confIndex := range confSpecs {
		confSpec := confSpecs[confIndex]

		v, err := app.evaluateConfig(jobCtx, confType, confSpec, confCtx, g)
		if err != nil {
			return cty.DynamicVal, err
		}

		confFields[confSpec.Name] = v
	}

	return cty.ObjectVal(confFields), nil
}

func (app *App) evaluateConfig(jobCtx *JobContext, confType string, confSpec Config, confCtx *hcl2.EvalContext, g func(map[string]interface{}) (map[string]interface{}, error)) (cty.Value, error) {
	merged := map[string]interface{}{}

	for sourceIdx := range confSpec.Sources {
		sourceSpec := confSpec.Sources[sourceIdx]

		fragments, err := app.loadConfigSource(jobCtx, confCtx, sourceSpec)
		if err != nil {
			return cty.DynamicVal, xerrors.Errorf("%s %q: source %d: %w", confType, confSpec.Name, sourceIdx, err)
		}

		for _, f := range fragments {
			yamlData := f.data
			format := f.format
			key := f.key

			m := map[string]interface{}{}

			switch format {
			case FormatYAML:
				if err := yaml.Unmarshal(yamlData, &m); err != nil {
					return cty.DynamicVal, xerrors.Errorf("unmarshalling yaml: %w", err)
				}
			case FormatText:
				if key == "" {
					return cty.DynamicVal, fmt.Errorf("`key` must be specified for `text`-formatted source at %d", sourceIdx)
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
				return cty.DynamicVal, fmt.Errorf("format %q is not implemented yet. It must be \"yaml\" or omitted", format)
			}

			if err := mergo.Merge(&merged, m, mergo.WithOverride, mergo.WithOverwriteWithEmptyValue); err != nil {
				return cty.DynamicVal, xerrors.Errorf("merging maps: %w", err)
			}
		}
	}

	if g != nil {
		r, err := g(merged)
		if err != nil {
			return cty.DynamicVal, err
		}

		merged = r
	}

	yamlData, err := yaml.Marshal(merged)
	if err != nil {
		return cty.DynamicVal, xerrors.Errorf("generating yaml: %w", err)
	}

	ty, err := ctyyaml.ImpliedType(yamlData)
	if err != nil {
		return cty.DynamicVal, xerrors.Errorf("determining type of %s: %w", string(yamlData), err)
	}

	v, err := ctyyaml.Unmarshal(yamlData, ty)
	if err != nil {
		return cty.DynamicVal, xerrors.Errorf("unmarshalling %s: %w", string(yamlData), err)
	}

	return v, nil
}

//nolint:unused
func (app *App) addConfigsAndVariablesDeprecated(jobCtx *JobContext, varSpecs []Variable, configs []Config, secrets []Config, g func(m map[string]interface{}) (map[string]interface{}, error)) (*hcl2.EvalContext, error) {
	conf, err := app.getConfigs(jobCtx, "config", configs, nil)
	if err != nil {
		return nil, err
	}

	secJobCtx := jobCtx.WithVariable("conf", conf).Ptr()

	sec, err := app.getConfigs(secJobCtx, "secret", secrets, g)
	if err != nil {
		return nil, err
	}

	varJobCtx := secJobCtx.WithVariable("sec", sec)

	varCtx, err := setVariables(varJobCtx.evalContext, varSpecs)
	if err != nil {
		return nil, err
	}

	return varCtx, nil
}

//nolint:gocyclo
func (app *App) addConfigsAndVariables(jobCtx *JobContext, varSpecs []Variable, confSpecs []Config, secSpecs []Config, g func(m map[string]interface{}) (map[string]interface{}, error)) (*hcl2.EvalContext, error) {
	ctx := jobCtx.evalContext

	type node struct {
		config   *Config
		secret   *Config
		variable *Variable
	}

	d := dag.New()

	nodes := map[string]node{}

	dynamicDependencyName := func(v hcl2.Traversal) *string {
		if !v.IsRelative() && (v.RootName() == "conf" || v.RootName() == "var" || v.RootName() == "sec") {
			if r, ok := v.SimpleSplit().Rel[0].(hcl2.TraverseAttr); ok {
				id := fmt.Sprintf("%s.%s", v.RootName(), r.Name)

				return &id
			}
		}

		return nil
	}

	for i := range varSpecs {
		v := varSpecs[i]

		id := fmt.Sprintf("var.%s", v.Name)

		nodes[id] = node{
			variable: &v,
		}

		var deps []string

		for _, v := range v.Value.Variables() {
			if d := dynamicDependencyName(v); d != nil {
				deps = append(deps, *d)
			}
		}

		d.Add(id, dag.Dependencies(deps))
	}

	for i := range confSpecs {
		c := confSpecs[i]

		id := fmt.Sprintf("conf.%s", c.Name)

		nodes[id] = node{
			config: &c,
		}

		var deps []string

		for _, s := range c.Sources {
			content, err := loadConfigSourceContent(s)
			if err != nil {
				return nil, xerrors.Errorf("loading config %s's %s source: %w", c.Name, s.Type, err)
			}

			for _, a := range content.Attributes {
				for _, v := range a.Expr.Variables() {
					if d := dynamicDependencyName(v); d != nil {
						deps = append(deps, *d)
					}
				}
			}
		}

		d.Add(id, dag.Dependencies(deps))
	}

	for i := range secSpecs {
		c := secSpecs[i]

		id := fmt.Sprintf("sec.%s", c.Name)

		nodes[id] = node{
			secret: &c,
		}

		var deps []string

		for _, s := range c.Sources {
			content, err := loadConfigSourceContent(s)
			if err != nil {
				return nil, xerrors.Errorf("loading config %s's %s source: %w", c.Name, s.Type, err)
			}

			for _, a := range content.Attributes {
				for _, v := range a.Expr.Variables() {
					if d := dynamicDependencyName(v); d != nil {
						deps = append(deps, *d)
					}
				}
			}
		}

		d.Add(id, dag.Dependencies(deps))
	}

	top, err := d.Sort()
	if err != nil {
		return nil, xerrors.Errorf("resolving dependencies among variables and configs: %w", err)
	}

	varFields := map[string]cty.Value{}
	confFields := map[string]cty.Value{}
	secFields := map[string]cty.Value{}

	//nolint:nestif
	for _, wave := range top {
		ctx.Variables["var"] = cty.ObjectVal(varFields)
		ctx.Variables["conf"] = cty.ObjectVal(confFields)
		ctx.Variables["sec"] = cty.ObjectVal(secFields)

		for _, info := range wave {
			node, ok := nodes[info.Id]
			if !ok {
				return nil, xerrors.Errorf("missing node %s", info.Id)
			}

			if node.config != nil {
				r, err := app.evaluateConfig(jobCtx, "config", *node.config, ctx, nil)
				if err != nil {
					return nil, err
				}

				confFields[node.config.Name] = r
			} else if node.secret != nil {
				r, err := app.evaluateConfig(jobCtx, "secret", *node.secret, ctx, g)
				if err != nil {
					return nil, err
				}

				secFields[node.secret.Name] = r
			} else if node.variable != nil {
				r, err := evaluateVariable(ctx, *node.variable)
				if err != nil {
					return nil, xerrors.Errorf("%w", err)
				}

				varFields[node.variable.Name] = r
			} else {
				panic(fmt.Errorf("invalid state: either config or variable must be set in node: %+v", node))
			}
		}
	}

	ctx.Variables["var"] = cty.ObjectVal(varFields)
	ctx.Variables["conf"] = cty.ObjectVal(confFields)
	ctx.Variables["sec"] = cty.ObjectVal(secFields)

	return ctx, nil
}

func setVariables(varCtx *hcl2.EvalContext, varSpecs []Variable) (*hcl2.EvalContext, error) {
	varFields := map[string]cty.Value{}

	for _, varSpec := range varSpecs {
		val, err := evaluateVariable(varCtx, varSpec)
		if err != nil {
			return nil, err
		}

		varFields[varSpec.Name] = val

		varCtx.Variables["var"] = cty.ObjectVal(varFields)
	}

	return varCtx, nil
}

func evaluateVariable(varCtx *hcl2.EvalContext, varSpec Variable) (cty.Value, error) {
	var tpe cty.Type

	tv, _ := varSpec.Type.Value(nil)

	if !tv.IsNull() {
		var diags hcl2.Diagnostics

		tpe, diags = typeexpr.TypeConstraint(varSpec.Type)
		if diags != nil {
			return cty.DynamicVal, diags
		}
	}

	//nolint:nestif
	if tpe.IsListType() && tpe.ListElementType().Equals(cty.String) {
		var v []string
		if err := gohcl2.DecodeExpression(varSpec.Value, varCtx, &v); err != nil {
			return cty.DynamicVal, err
		}

		if vty, err := gocty.ImpliedType(v); err != nil {
			return cty.DynamicVal, err
		} else if vty != tpe {
			return cty.DynamicVal, fmt.Errorf("unexpected type of option. want %q, got %q", tpe.FriendlyNameForConstraint(), vty.FriendlyName())
		}

		val, err := gocty.ToCtyValue(v, tpe)
		if err != nil {
			return cty.DynamicVal, err
		}

		return val, nil
	}

	var v cty.Value

	if err := gohcl2.DecodeExpression(varSpec.Value, varCtx, &v); err != nil {
		return cty.DynamicVal, err
	}

	vty := v.Type()

	if !tv.IsNull() && !vty.Equals(tpe) {
		return cty.DynamicVal, fmt.Errorf("unexpected type of value for variable. want %q, got %q", tpe.FriendlyNameForConstraint(), vty.FriendlyName())
	}

	return v, nil
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
