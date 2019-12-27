package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestExamples(t *testing.T) {
	testcases := []struct {
		args      []string
		cmd       string
		dir       string
		expectErr string
		expectOut string
	}{
		{
			cmd:  "complex",
			args: []string{"complex", "main", "--opt1", "OPT1"},
			dir:  "./examples/complex",
		},
		{
			cmd:       "simple",
			args:      []string{"simple", "--opt1", "OPT1"},
			dir:       "./examples/simple",
			expectErr: "unknown flag: --opt1",
		},
		{
			cmd: "simple",
			// TODO this should fail. Impelemnt shell runner
			args: []string{"simple", "app", "deploy", "--namespace", "ns1"},
			dir:  "./examples/simple",
			expectErr: "command \"bash -c     kubectl -n ns1 apply -f examples/simple/manifests/\n\": exit status 1",
		},
		{
			cmd:  "simple",
			args: []string{"simple", "app", "deploy", "--namespace", "default"},
			dir:  "./examples/simple",
		},
		{
			cmd:  "",
			args: []string{"variant", "test"},
			dir:  "./examples/simple",
		},
		{
			cmd:  "kubectl",
			args: []string{"kubectl", "apply", "--namespace", "default", "-f", "examples/simple/manifests/"},
			dir:  "./examples/simple/mocks/kubectl",
		},
		{
			cmd:       "rubyrunner",
			args:      []string{"rubyrunner", "test1"},
			dir:       "./examples/rubyrunner",
			expectOut: "TEST\n",
		},
		{
			cmd:       "rubyrunner",
			args:      []string{"rubyrunner", "test2"},
			dir:       "./examples/rubyrunner",
			expectOut: "TEST\n",
		},
		{
			cmd:       "rubyrunner",
			args:      []string{"rubyrunner", "test3"},
			dir:       "./examples/rubyrunner",
			expectOut: "TEST\n",
		},
	}

	for i := range testcases {
		tc := testcases[i]
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			outRead, outWrite := io.Pipe()
			m := Main{
				Stdout: outWrite,
				Stderr: os.Stderr,
				Args:   tc.args,
				Getenv: func(name string) string {
					switch name {
					case "VARIANT_NAME":
						return tc.cmd
					case "VARIANT_DIR":
						return tc.dir
					default:
						panic(fmt.Sprintf("Unexpected call to getenv %q", name))
					}
				},
			}
			var err error

			go func() {
				err = m.Run()
				outWrite.Close()
			}()

			buf := new(bytes.Buffer)
			buf.ReadFrom(outRead)
			out := buf.String()

			if tc.expectErr != "" {
				if err == nil {
					t.Fatalf("Expected error didn't occur")
				} else {
					if err.Error() != tc.expectErr {
						t.Fatalf("Unexpected error: want %q, got %q", tc.expectErr, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Fatalf("%+v", err)
				}
			}

			if tc.expectOut != "" {
				if tc.expectOut != out {
					t.Errorf("unexpected output: want %q, got %q", tc.expectOut, out)
				}
			}
		})
	}
}
