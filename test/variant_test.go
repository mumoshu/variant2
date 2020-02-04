package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	variant "github.com/mumoshu/variant2"
)

// Building the binary with `go build -o myapp main.go`
// and running
//   ./myapp test
// should produce:
//    HELLO WORLD

func TestMustEval(t *testing.T) {
	source := `
job "test" {
  exec {
    command = "echo"
    args = ["HELLO WORLD"]
  }
}
`
	err := variant.MustEval("myapp", source).Run([]string{"test"})

	if err != nil {
		panic(err)
	}

	var verr variant.Error

	var code int

	if err != nil {
		if ok := errors.As(err, &verr); ok {
			code = verr.ExitCode
		} else {
			code = 1
		}
	} else {
		code = 0
	}

	if code != 0 {
		t.Errorf("unexpected code: %d", code)
	}
}

func TestNewFile(t *testing.T) {
	myapp, err := variant.Load("../examples/simple/simple.variant")
	if err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	if err := myapp.Run([]string{"app", "deploy", "--namespace=default"}, variant.RunOptions{
		Stdout: stdout,
		Stderr: stderr,
	}); err != nil {
		t.Fatal(err)
	}

	var verr variant.Error

	var code int

	if err != nil {
		if ok := errors.As(err, &verr); ok {
			code = verr.ExitCode
		} else {
			code = 1
		}
	} else {
		code = 0
	}

	if code != 0 {
		t.Errorf("unexpected code: %d", code)
	}
}

func TestExtensionWithGo(t *testing.T) {
	myapp, err := variant.Load("../examples/simple/simple.variant")
	if err != nil {
		t.Fatal(err)
	}

	// outWriter and errWriter are automatically closed by Variant core after the calling anonymous func
	// to avoid leaking
	myapp.Add(
		variant.Job{
			Name:        "foo bar",
			Description: "foobar",
			Options: map[string]variant.Variable{
				"namespace": {
					Type:        variant.String,
					Description: "namespace",
				},
			},
			Parameters: map[string]variant.Variable{
				"param1": {
					Type:        variant.String,
					Description: "param1",
				},
			},
			Run: func(ctx context.Context, s variant.State) error {
				v, ok := s.Options["namespace"]

				if !ok {
					return fmt.Errorf("missing option %q", "namespace")
				}

				ns := v.(string)

				out, stdoutW := variant.Pipe()
				errs, stderrW := variant.Pipe()

				defer s.Stdout.Close()
				defer s.Stderr.Close()

				subst := variant.State{
					Parameters: map[string]interface{}{},
					Options:    map[string]interface{}{"namespace": ns},
					Stdout:     stdoutW,
					Stderr:     stderrW,
				}
				j, err := myapp.Job("app deploy", subst)

				if err != nil {
					return err
				}

				if err := j(ctx); err != nil {
					return err
				}

				o, err := out()
				if err != nil {
					return err
				}

				if _, err := s.Stdout.Write([]byte("OUTPUT: " + o.String())); err != nil {
					return err
				}

				e, err := errs()
				if err != nil {
					return err
				}

				if _, err := s.Stderr.Write([]byte("ERROR: " + e.String())); err != nil {
					return err
				}

				return nil
			},
		})

	getStdout, stdout := variant.Pipe()
	getStderr, stderr := variant.Pipe()

	jr, err := myapp.Job("foo bar", variant.State{
		Stdout:     stdout,
		Stderr:     stderr,
		Parameters: map[string]interface{}{},
		Options:    map[string]interface{}{"namespace": "default"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := jr(context.TODO()); err != nil {
		t.Fatal(err)
	}

	outs, err := getStdout()
	if err != nil {
		t.Fatal(err)
	}

	outStr := outs.String()
	if outStr != "<nil>" {
		t.Errorf("unexpected stdout: got %q", outStr)
	}

	errs, err := getStderr()
	if err != nil {
		t.Fatal(err)
	}

	errStr := errs.String()
	if errStr != "<nil>" {
		t.Errorf("unexpected stderr: got %q", errStr)
	}

	var verr variant.Error

	var code int

	if err != nil {
		if ok := errors.As(err, &verr); ok {
			code = verr.ExitCode
		} else {
			code = 1
		}
	} else {
		code = 0
	}

	if code != 0 {
		t.Errorf("unexpected code: %d", code)
	}
}

func TestNewDir(t *testing.T) {
	myapp, err := variant.Load("../examples/simple")
	if err != nil {
		panic(err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	if err := myapp.Run([]string{"app", "deploy", "--namespace=default"}, variant.RunOptions{
		Stdout: stdout,
		Stderr: stderr,
	}); err != nil {
		t.Fatal(err)
	}

	var verr variant.Error

	var code int

	if err != nil {
		if ok := errors.As(err, &verr); ok {
			code = verr.ExitCode
		} else {
			code = 1
		}
	} else {
		code = 0
	}

	if code != 0 {
		t.Errorf("unexpected code: %d", code)
	}
}

func TestNewDirCobra(t *testing.T) {
	myapp, err := variant.Load("../examples/simple")
	if err != nil {
		panic(err)
	}

	// Returns the *cobra.Command
	cmd, err := myapp.Cobra()
	if err != nil {
		t.Fatal(err)
	}

	// You can add any command to the root command with:
	//   cmd.AddCommand(...)
	// See the documentation of cobra for more information.

	cmd.SetArgs([]string{"app", "deploy", "--namespace=default"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// variant.(Must)Eval creates a Variant command from the virtual file name and the source code written in the Variant DSL
// variant.(Must)Load creates a Variant command from a file or a directory
