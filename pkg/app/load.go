package app

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/mumoshu/variant2/pkg/conf"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/zclconf/go-cty/cty"
)

type hcl2Loader struct {
	Parser *hclparse.Parser
}

type configurable struct {
	Body hcl.Body
}

func (l hcl2Loader) loadFile(filenames ...string) (*configurable, map[string]*hcl.File, error) {
	srcs := map[string][]byte{}

	for _, filename := range filenames {
		src, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, nil, err
		}

		srcs[filename] = src
	}

	return l.loadSources(srcs)
}

func (l hcl2Loader) loadSources(srcs map[string][]byte) (*configurable, map[string]*hcl.File, error) {
	nameToFiles := map[string]*hcl.File{}

	var files []*hcl.File

	var diags hcl.Diagnostics

	for filename, src := range srcs {
		var f *hcl.File

		var ds hcl.Diagnostics

		if strings.HasSuffix(filename, ".json") {
			f, ds = l.Parser.ParseJSON(src, filename)
		} else {
			f, ds = l.Parser.ParseHCL(src, filename)
		}

		nameToFiles[filename] = f
		files = append(files, f)
		diags = append(diags, ds...)
	}

	if diags.HasErrors() {
		return nil, nameToFiles, diags
	}

	body := hcl.MergeFiles(files)

	return &configurable{
		Body: body,
	}, nameToFiles, nil
}

func (t *configurable) HCL2Config() (*HCL2Config, error) {
	config := &HCL2Config{}

	ctx := &hcl.EvalContext{
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

	diags := gohcl.DecodeBody(t.Body, ctx, config)
	if diags.HasErrors() {
		return config, diags
	}

	return config, nil
}

func New(dir string) (*App, error) {
	nameToFiles, cc, err := newConfigFromDir(dir)

	app := &App{
		Files: nameToFiles,
		Trace: os.Getenv("VARIANT_TRACE"),
	}

	if err != nil {
		return app, err
	}

	return newApp(app, cc, dir, true)
}

func NewFromFile(file string) (*App, error) {
	nameToFiles, cc, err := newConfigFromFiles([]string{file})

	app := &App{
		Files: nameToFiles,
	}

	if err != nil {
		return app, err
	}

	dir := filepath.Dir(file)

	return newApp(app, cc, dir, true)
}

func NewFromSources(srcs map[string][]byte) (*App, error) {
	nameToFiles, _, cc, err := newConfigFromSources(srcs)

	app := &App{
		Files: nameToFiles,
	}

	if err != nil {
		return app, err
	}

	return newApp(app, cc, "", false)
}

func newConfigFromDir(dirPathOrURL string) (map[string]*hcl.File, *HCL2Config, error) {
	var dir string

	s := strings.Split(dirPathOrURL, "::")

	if len(s) > 1 {
		forcePrefix := s[0]

		u, err := url.Parse(s[1])
		if err != nil {
			return nil, nil, err
		}

		remote, err := depresolver.New(depresolver.Home(".variant2/cache"))
		if err != nil {
			return nil, nil, err
		}

		us := forcePrefix + "::" + u.String()

		cacheDir, err := remote.ResolveDir(us)
		if err != nil {
			return nil, nil, err
		}

		dir = cacheDir
	} else {
		dir = dirPathOrURL
	}

	files, err := conf.FindVariantFiles(dir)
	if err != nil {
		return map[string]*hcl.File{}, nil, fmt.Errorf("failed to get %s files: %v", conf.VariantFileExt, err)
	}

	return newConfigFromFiles(files)
}

func newConfigFromFiles(files []string) (map[string]*hcl.File, *HCL2Config, error) {
	l := &hcl2Loader{
		Parser: hclparse.NewParser(),
	}

	c, nameToFiles, err := l.loadFile(files...)

	if err != nil {
		return nameToFiles, nil, err
	}

	cc, err := c.HCL2Config()

	return nameToFiles, cc, err
}

func newConfigFromSources(srcs map[string][]byte) (map[string]*hcl.File, hcl.Body, *HCL2Config, error) {
	l := &hcl2Loader{
		Parser: hclparse.NewParser(),
	}

	c, nameToFiles, err := l.loadSources(srcs)

	if err != nil {
		var body hcl.Body
		if c != nil {
			body = c.Body
		}

		return nameToFiles, body, nil, err
	}

	cc, err := c.HCL2Config()

	return nameToFiles, c.Body, cc, err
}

func newApp(app *App, cc *HCL2Config, importBaseDir string, enableImports bool) (*App, error) {
	jobs := append([]JobSpec{cc.JobSpec}, cc.Jobs...)

	var conf *HCL2Config

	jobByName := map[string]JobSpec{}
	for _, j := range jobs {
		jobByName[j.Name] = j

		if j.Import != nil {
			if !enableImports {
				return nil, fmt.Errorf("[bug] Imports are disable in the embedded mode")
			}

			var d string

			if strings.Contains(*j.Import, ":") {
				d = *j.Import
			} else {
				d = filepath.Join(importBaseDir, *j.Import)
			}

			a, err := New(d)

			if err != nil {
				return nil, err
			}

			importedJobs := append([]JobSpec{a.Config.JobSpec}, a.Config.Jobs...)
			for _, importedJob := range importedJobs {
				var newJobName string
				if j.Name == "" {
					newJobName = importedJob.Name
				} else {
					newJobName = fmt.Sprintf("%s %s", j.Name, importedJob.Name)
				}

				jobByName[newJobName] = importedJob

				if j.Name == "" && importedJob.Name == "" {
					conf = a.Config
				}
			}
		}
	}

	if conf == nil {
		conf = cc
	}

	app.Config = conf

	app.JobByName = jobByName

	return app, nil
}
