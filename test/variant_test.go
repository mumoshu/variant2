package test

import (
	"bytes"
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

	//myapp.Job("foo bar", func(outWriter, errWriter io.WriteCloser, args func(interface{}) error) error {
	//	defer outWriter.Close()
	//	defer errWriter.Close()
	//
	//	parsed := struct{
	//		A string
	//		B string
	//	}{}
	//
	//	if err := args(&parsed); err != nil {
	//		return err
	//	}
	//
	//	if _, err := outWriter.Write([]byte("OUTPUT")); err != nil {
	//		return err
	//	}
	//
	//	if _, err := errWriter.Write([]byte("ERROR")); err != nil {
	//		return err
	//	}
	//})

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

	if cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}


// variant.(Must)Eval creates a Variant command from the virtual file name and the source code written in the Variant DSL
// variant.(Must)Load creates a Variant command from a file or a directory
