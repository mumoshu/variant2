package main

import (
	"fmt"
	"os"
	"testing"
)

func TestExamples(t *testing.T) {
	testcases := []struct {
		args      []string
		cmd       string
		dir       string
		expectErr string
	}{
		{
			cmd:  "complex",
			args: []string{"complex", "--opt1", "OPT1"},
			dir:  "./examples/complex",
		},
		{
			cmd:       "simple",
			args:      []string{"simple", "--opt1", "OPT1"},
			dir:       "./examples/simple",
			expectErr: "unknown flag: --opt1",
		},
		{
			cmd:  "simple",
			args: []string{"simple", "app", "deploy", "--namespace", "ns1"},
			dir:  "./examples/simple",
		},
		{
			cmd:  "kubectl",
			args: []string{"kubectl", "apply", "--namespace", "ns1", "-f", "manifests/"},
			dir:  "./examples/simple/mocks/kubectl",
		},
		{
			cmd:  "kubectl",
			args: []string{"kubectl", "apply", "-aah"},
			dir:  "./examples/simple/mocks/kubectl",
		},
	}

	for i := range testcases {
		tc := testcases[i]
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			m := Main{
				Stdout: os.Stdout,
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
			err := m.Run()

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
		})
	}
}
