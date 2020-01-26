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
		subject     string
		args        []string
		variantName string
		variantDir  string
		wd          string
		expectErr   string
		expectOut   string
	}{
		{
			variantName: "complex",
			args:        []string{"variant", "main", "--opt1", "OPT1"},
			variantDir:  "./examples/complex",
		},
		{
			variantName: "simple",
			args:        []string{"variant", "--opt1", "OPT1"},
			variantDir:  "./examples/simple",
			expectErr:   "unknown flag: --opt1",
		},
		{
			variantName: "simple",
			args:        []string{"variant", "app", "deploy", "--namespace", "ns1"},
			variantDir:  "./examples/simple",
			expectErr:   "command \"bash -c     kubectl -n ns1 apply -f examples/simple/manifests/\n\": exit status 1",
		},
		{
			variantName: "simple",
			args:        []string{"variant", "app", "deploy", "--namespace", "default"},
			variantDir:  "./examples/simple",
		},
		{
			args:       []string{"variant", "run", "app", "deploy", "--namespace", "ns1"},
			variantDir: "./examples/simple",
			expectErr:  "command \"bash -c     kubectl -n ns1 apply -f examples/simple/manifests/\n\": exit status 1",
		},
		{
			args:       []string{"variant", "run", "app", "deploy", "--namespace", "default"},
			variantDir: "./examples/simple",
		},
		{
			subject:    "module",
			args:       []string{"variant", "run", "test"},
			variantDir: "./examples/module",
		},
		{
			subject:    "module_test",
			args:       []string{"variant", "test"},
			variantDir: "./examples/module",
		},
		{
			variantName: "",
			args:        []string{"variant", "test"},
			variantDir:  "./examples/simple",
		},
		{
			variantName: "",
			args:        []string{"variant", "test"},
			wd:          "./examples/simple",
		},
		{
			subject:     "config",
			variantName: "",
			args:        []string{"variant", "test"},
			wd:          "./examples/config",
		},
		{
			subject:     "secret",
			variantName: "",
			args:        []string{"variant", "test"},
			wd:          "./examples/secret",
		},
		{
			subject:     "concurrency",
			variantName: "",
			args:        []string{"variant", "test"},
			wd:          "./examples/concurrency",
		},
		{
			variantName: "kubectl",
			args:        []string{"variant", "apply", "--namespace", "default", "-f", "examples/simple/manifests/"},
			variantDir:  "./examples/simple/mocks/kubectl",
		},
		{
			variantName: "rubyrunner",
			args:        []string{"variant", "test1"},
			variantDir:  "./examples/rubyrunner",
			expectOut:   "TEST\n",
		},
		{
			variantName: "rubyrunner",
			args:        []string{"variant", "test2"},
			variantDir:  "./examples/rubyrunner",
			expectOut:   "TEST\n",
		},
		{
			variantName: "rubyrunner",
			args:        []string{"variant", "test3"},
			variantDir:  "./examples/rubyrunner",
			expectOut:   "TEST\n",
		},
		{
			subject:     "logcollection",
			variantName: "",
			args:        []string{"variant", "test"},
			wd:          "./examples/advanced/logcollection",
		},
		{
			subject:     "options",
			variantName: "",
			args:        []string{"variant", "test"},
			wd:          "./examples/options",
		},
	}

	for i := range testcases {
		tc := testcases[i]
		t.Run(fmt.Sprintf("%d: %s", i, tc.subject), func(t *testing.T) {
			outRead, outWrite := io.Pipe()
			m := Main{
				Stdout: outWrite,
				Stderr: os.Stderr,
				Args:   tc.args,
				Getenv: func(name string) string {
					switch name {
					case "VARIANT_NAME":
						return tc.variantName
					case "VARIANT_DIR":
						return tc.variantDir
					default:
						panic(fmt.Sprintf("Unexpected call to getenv %q", name))
					}
				},
				Getwd: func() (string, error) {
					if tc.wd != "" {
						return tc.wd, nil
					}
					return "", fmt.Errorf("Unexpected call to getw")
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
