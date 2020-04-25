package variant

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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
			subject:    "variant run app deploy on simple example w/ ns1",
			args:       []string{"variant", "run", "app", "deploy", "--namespace", "ns1"},
			variantDir: "./examples/simple",
			expectErr:  "command \"bash -c     kubectl -n ns1 apply -f examples/simple/manifests/\n\": exit status 1",
		},
		{
			subject:    "variant run app deploy on simple example w/ default",
			args:       []string{"variant", "run", "app", "deploy", "--namespace", "default"},
			variantDir: "./examples/simple",
		},
		{
			subject:    "module",
			args:       []string{"variant", "run", "test"},
			variantDir: "./examples/module",
		},
		{
			subject:    "defaults_test",
			args:       []string{"variant", "test"},
			variantDir: "./examples/defaults",
		},
		{
			subject:    "module_test",
			args:       []string{"variant", "test"},
			variantDir: "./examples/module",
		},
		{
			subject:    "depends_on_test",
			args:       []string{"variant", "test"},
			variantDir: "./examples/depends_on",
		},
		{
			subject:     "variant test on simple example",
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
			subject:     "kubectl mock apply",
			variantName: "kubectl",
			args:        []string{"variant", "apply", "--namespace", "default", "-f", "examples/simple/manifests/"},
			variantDir:  "./examples/simple/mocks/kubectl",
		},
		{
			subject:     "rubyrunner test1",
			variantName: "rubyrunner",
			args:        []string{"variant", "test1"},
			variantDir:  "./examples/rubyrunner",
			expectOut:   "TEST\n",
		},
		{
			subject:     "rubyrunner test2",
			variantName: "rubyrunner",
			args:        []string{"variant", "test2"},
			variantDir:  "./examples/rubyrunner",
			expectOut:   "TEST\n",
		},
		{
			subject:     "rubyrunner test3",
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
			subject:     "import",
			variantName: "",
			args:        []string{"variant", "test"},
			wd:          "./examples/advanced/import",
		},
		{
			subject:     "options",
			variantName: "",
			args:        []string{"variant", "test"},
			wd:          "./examples/options",
		},
		{
			subject:     "options-json",
			variantName: "",
			args:        []string{"variant", "test"},
			wd:          "./examples/options-json",
		},
		{
			subject: "shebang",
			args:    []string{"variant", "./test/shebang/myapp/myapp", "test", "--int1", "1", "--ints1", "1,2", "--str1", "a", "--strs1", "b,c"},
			wd:      "./test",
		},
		{
			subject: "examples/issues/sweetops-CFFQ9GFB5-p1586798062189700",
			args:    []string{"variant", "run", "example", "echo foo", "echo bar", "-p", "myproj", "-t", "mytenant"},
			wd:      "./examples/issues/sweetops-CFFQ9GFB5-p1586798062189700",
		},
		{
			subject: "examples/issues/cant-convert-go-str-to-bool",
			args:    []string{"variant", "test"},
			wd:      "./examples/issues/cant-convert-go-str-to-bool",
		},
		{
			subject: "examples/exec",
			args:    []string{"variant", "test"},
			wd:      "./examples/exec",
		},
		{
			subject: "examples/variables",
			args:    []string{"variant", "test"},
			wd:      "./examples/variables",
		},
		{
			subject: "examples/globals",
			args:    []string{"variant", "test"},
			wd:      "./examples/globals",
		},
		{
			subject: "examples/testing",
			args:    []string{"variant", "test"},
			wd:      "./examples/testing",
		},
		{
			subject: "examples/advaned/terraform-and-helmfile-wrapper",
			args:    []string{"variant", "test"},
			wd:      "./examples/advanced/terraform-and-helmfile-wrapper",
		},
	}

	for i := range testcases {
		tc := testcases[i]
		t.Run(fmt.Sprintf("%d: %s", i, tc.subject), func(t *testing.T) {
			outRead, outWrite := io.Pipe()
			m := Init(Main{
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
			})
			var err error

			go func() {
				err = m.Run()
				outWrite.Close()
			}()

			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(outRead); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			out := buf.String()

			if tc.expectErr != "" {
				if err == nil {
					t.Fatalf("Expected error didn't occur")
				} else if err.Error() != tc.expectErr {
					t.Fatalf("Unexpected error: want %q, got %q", tc.expectErr, err.Error())
				}
			} else if err != nil {
				t.Fatalf("%+v", err)
			}

			if tc.expectOut != "" {
				if tc.expectOut != out {
					t.Errorf("unexpected output: want %q, got %q", tc.expectOut, out)
				}
			}
		})
	}
}

func TestExport(t *testing.T) {
	testcases := []struct {
		subject    string
		exportArgs []string
		testArgs   []string
		srcDir     string
		dstDir     string
		expectErr  string
		expectOut  string
	}{
		{
			subject:    "simple",
			exportArgs: []string{"variant", "export", "shim"},
			testArgs:   []string{"test", "--int1", "1", "--ints1", "1,2", "--str1", "a", "--strs1", "b,c"},
			srcDir:     "./test/export/simple/src",
			dstDir:     "./test/export/simple/dst",
		},
	}

	for i := range testcases {
		tc := testcases[i]
		t.Run(fmt.Sprintf("%d: %s", i, tc.subject), func(t *testing.T) {
			outRead, outWrite := io.Pipe()
			m := Init(Main{
				Stdout: outWrite,
				Stderr: os.Stderr,
				Args:   append(append([]string{}, tc.exportArgs...), tc.srcDir, tc.dstDir),
				Getenv: func(name string) string {
					switch name {
					case "VARIANT_NAME":
						return ""
					case "VARIANT_DIR":
						return ""
					default:
						panic(fmt.Sprintf("Unexpected call to getenv %q", name))
					}
				},
				Getwd: func() (string, error) {
					if tc.srcDir != "" {
						return tc.srcDir, nil
					}
					return "", fmt.Errorf("Unexpected call to getw")
				},
			})
			var err error

			go func() {
				err = m.Run()
				outWrite.Close()
			}()

			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(outRead); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			out := buf.String()

			if tc.expectErr != "" {
				if err == nil {
					t.Fatalf("Expected error didn't occur")
				} else if err.Error() != tc.expectErr {
					t.Fatalf("Unexpected error: want %q, got %q", tc.expectErr, err.Error())
				}
			} else if err != nil {
				t.Fatalf("%+v", err)
			}

			if tc.expectOut != "" {
				if tc.expectOut != out {
					t.Errorf("unexpected output: want %q, got %q", tc.expectOut, out)
				}
			}

			base := filepath.Base(tc.dstDir)
			shimPath := fmt.Sprintf("%s/%s", tc.dstDir, base)
			args := []string{"-c", strings.Join(append([]string{shimPath}, tc.testArgs...), " ")}
			cmd := exec.Command("/bin/bash", args...)
			if err := cmd.Run(); err != nil {
				t.Fatalf("failed to exec %s: %v", shimPath, err)
			}
		})
	}
}

func TestExec(t *testing.T) {
	testcases := []struct {
		subject string
		testCmd []string
		err     string
		out     string
	}{
		{
			subject: "shebang_test",
			testCmd: []string{
				"./test/shebang/myapp/myapp", "test", "--int1", "1", "--ints1", "1,2", "--str1", "a", "--strs1", "b,c",
			},
			out: `1 1 2 a b|c
`,
		},
		{
			subject: "shebang_test_usage",
			testCmd: []string{"./test/shebang/myapp/myapp", "test"},
			err:     `exit status 1`,
			out: `Error: required flag(s) "int1", "ints1", "str1", "strs1" not set
Usage:
  myapp test [flags]

Flags:
  -h, --help   help for test

Global Flags:
      --int1 int        
      --ints1 ints      
      --str1 string     
      --strs1 strings

`,
		},
		{
			subject: "shebang_usage",
			testCmd: []string{"./test/shebang/myapp/myapp"},
			err:     `exit status 1`,
			out: `Error: required flag(s) "int1", "ints1", "str1", "strs1" not set
Usage:
  myapp [flags]
  myapp [command]

Available Commands:
  help        Help about any command
  test        

Flags:
  -h, --help            help for myapp
      --int1 int        
      --ints1 ints      
      --str1 string     
      --strs1 strings

Use "myapp [command] --help" for more information about a command.

`,
		},
	}

	for i := range testcases {
		tc := testcases[i]
		t.Run(fmt.Sprintf("%d: %s", i, tc.subject), func(t *testing.T) {
			cmdline := strings.Join(tc.testCmd, " ")
			args := []string{"-c", cmdline}
			cmd := exec.Command("/bin/bash", args...)
			outBytes, err := cmd.CombinedOutput()
			out := string(outBytes)
			t.Log(out)
			if tc.err == "" {
				if err != nil {
					t.Errorf("failed to exec %s: %v", cmdline, err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error did not occur: want %q", tc.err)
				} else if tc.err != err.Error() {
					t.Errorf("unexpected error: want %q, got %v", tc.err, err)
				}
			}
			diff := cmp.Diff(tc.out, out)
			if tc.out != "" && diff != "" {
				t.Errorf("unexpected output: %s", diff)
			}
		})
	}
}
