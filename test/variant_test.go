package test

import (
	"bytes"
	"context"
	"errors"
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

	// outWriter and errWriter are automatically closed by Variant core after the calling anonymous func
	// to avoid leaking
	myapp.Add(
		variant.Job{
			Name:        "foo bar",
			Description: "foobar",
			Options: map[string]variant.Option{
				"namespace": {
					Type:        "string",
					Description: "namespace",
				},
			},
			Run: func(ctx context.Context, s variant.State) error {
				ns := s.Args["namespace"].(string)
				j, err := myapp.Job("bar baz", variant.JobOptions{Args: map[string]interface{}{"namespace": ns}})

				if err != nil {
					return err
				}

				if err := j.Run(ctx); err != nil {
					return err
				}

				if _, err := s.Stdout.Write([]byte("OUTPUT")); err != nil {
					return err
				}

				if _, err := s.Stderr.Write([]byte("ERROR: ")); err != nil {
					return err
				}

				return nil
			},
		})

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
