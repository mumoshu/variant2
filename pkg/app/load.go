package app

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/ext/typeexpr"

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

func loadFiles(filenames ...string) (map[string][]byte, error) {
	srcs := map[string][]byte{}

	for _, filename := range filenames {
		src, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}

		srcs[filename] = src
	}

	return srcs, nil
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

type Instance struct {
	Sources map[string][]byte
	Dir     string
}

type Setup func() (*Instance, error)

func FromFile(path string) Setup {
	return func() (*Instance, error) {
		srcs, err := loadFiles(path)
		if err != nil {
			return nil, err
		}

		dir := filepath.Dir(path)

		return &Instance{
			Sources: srcs,
			Dir:     dir,
		}, nil
	}
}

func FromDir(dir string) Setup {
	return func() (*Instance, error) {
		fs, err := findVariantFiles(dir)
		if err != nil {
			return nil, err
		}

		srcs, err := loadFiles(fs...)
		if err != nil {
			return nil, err
		}

		return &Instance{
			Sources: srcs,
			Dir:     dir,
		}, nil
	}
}

func FromSources(srcs map[string][]byte) Setup {
	return func() (*Instance, error) {
		return &Instance{
			Sources: srcs,
			Dir:     "",
		}, nil
	}
}

func New(setup Setup) (*App, error) {
	instance, err := setup()
	if err != nil {
		return nil, err
	}

	nameToFiles, cc, err := newConfigFromSources(instance.Sources)

	app := &App{
		Files: nameToFiles,
		Trace: os.Getenv("VARIANT_TRACE"),
	}

	if err != nil {
		return app, err
	}

	return newApp(app, cc, NewImportFunc(instance.Dir, func(path string) (*App, error) {
		return New(FromDir(path))
	}))
}

func NewImportFunc(importBaseDir string, f func(string) (*App, error)) func(string) (*App, error) {
	return func(dir string) (*App, error) {
		var d string

		if strings.Contains(dir, ":") {
			d = dir
		} else if dir[0] == '/' {
			d = dir
		} else {
			d = filepath.Join(importBaseDir, dir)
		}

		return f(d)
	}
}

func findVariantFiles(dirPathOrURL string) ([]string, error) {
	var dir string

	s := strings.Split(dirPathOrURL, "::")

	if len(s) > 1 {
		forcePrefix := s[0]

		u, err := url.Parse(s[1])
		if err != nil {
			return nil, err
		}

		remote, err := depresolver.New(depresolver.Home(".variant2/cache"))
		if err != nil {
			return nil, err
		}

		us := forcePrefix + "::" + u.String()

		var cacheDir string

		u2, err := depresolver.Parse(us)
		if err != nil {
			return nil, err
		}

		if u2.File != "" {
			cacheDir, err = remote.ResolveFile(us)
		} else {
			cacheDir, err = remote.ResolveDir(us)
		}

		if err != nil {
			return nil, err
		}

		dir = cacheDir
	} else {
		dir = dirPathOrURL
	}

	files, err := conf.FindVariantFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s files: %v", conf.VariantFileExt, err)
	}

	return files, nil
}

func newConfigFromSources(srcs map[string][]byte) (map[string]*hcl.File, *HCL2Config, error) {
	l := &hcl2Loader{
		Parser: hclparse.NewParser(),
	}

	c, nameToFiles, err := l.loadSources(srcs)

	if err != nil {
		return nameToFiles, nil, err
	}

	cc, err := c.HCL2Config()

	return nameToFiles, cc, err
}

func newApp(app *App, cc *HCL2Config, importDir func(string) (*App, error)) (*App, error) {
	jobs := append([]JobSpec{cc.JobSpec}, cc.Jobs...)

	var conf *HCL2Config

	var globals []JobSpec

	jobByName := map[string]JobSpec{}
	for _, j := range jobs {
		jobByName[j.Name] = j

		var importSources []string

		if j.Imports != nil {
			importSources = append(importSources, *j.Imports...)
		}

		if j.Import != nil {
			importSources = append(importSources, *j.Import)
		}

		if len(importSources) > 0 {
			for _, src := range importSources {
				a, err := importDir(src)

				if err != nil {
					return nil, err
				}

				importedJobs := append([]JobSpec{a.Config.JobSpec}, a.Config.Jobs...)
				for _, importedJob := range importedJobs {
					var newJobName string

					if importedJob.Name == "" {
						// Do not override global parameters and options.
						//
						// If the user-side has a global parameter/option that has the same name as the library-side,
						// their types MUST match.
						merged, err := mergeParamsAndOpts(importedJob, j)
						if err != nil {
							return nil, fmt.Errorf("merging globals: %w", err)
						}

						merged.Name = ""

						importedJob = *merged
					}

					if j.Name == "" {
						newJobName = importedJob.Name
					} else if importedJob.Name != "" {
						newJobName = fmt.Sprintf("%s %s", j.Name, importedJob.Name)
					} else {
						// Import the top-level job in the library as the non-top-level job on the user side
						newJobName = j.Name

						// And merge global parameters and options
						globals = append(globals, importedJob)
					}

					importedJob.Name = newJobName

					jobByName[newJobName] = importedJob

					if j.Name == "" && importedJob.Name == "" {
						conf = a.Config
					}
				}
			}
		}
	}

	root := jobByName[""]

	for _, g := range globals {
		merged, err := mergeParamsAndOpts(g, root)
		if err != nil {
			return nil, fmt.Errorf("merging globals: %w", err)
		}

		root = *merged
	}

	jobByName[""] = root

	if conf == nil {
		conf = cc
	}

	app.Config = conf

	app.Config.JobSpec = root

	app.JobByName = jobByName

	var newJobs []JobSpec

	for _, j := range app.JobByName {
		newJobs = append(newJobs, j)
	}

	app.Config.Jobs = newJobs

	return app, nil
}

func mergeParamsAndOpts(src JobSpec, dst JobSpec) (*JobSpec, error) {
	paramMap := map[string]Parameter{}
	optMap := map[string]OptionSpec{}

	for _, p := range dst.Parameters {
		paramMap[p.Name] = p
	}

	for _, o := range dst.Options {
		optMap[o.Name] = o
	}

	for _, p := range src.Parameters {
		if existing, exists := paramMap[p.Name]; !exists {
			paramMap[p.Name] = p
		} else {
			exTy, err := typeexpr.TypeConstraint(existing.Type)
			if err != nil {
				return nil, fmt.Errorf("parsing parameter type: %w", err)
			}
			toTy, err := typeexpr.TypeConstraint(p.Type)
			if err != nil {
				return nil, fmt.Errorf("parsing parameter type: %w", err)
			}
			if exTy != toTy {
				return nil, fmt.Errorf("imported job %q has incompatible parameter %q: needs type of %v, encountered %v", src.Name, p.Name, exTy.GoString(), toTy.GoString())
			}
		}
	}

	for _, o := range src.Options {
		if existing, exists := optMap[o.Name]; !exists {
			optMap[o.Name] = o
		} else {
			exTy, err := typeexpr.TypeConstraint(existing.Type)
			if err != nil {
				return nil, fmt.Errorf("parsing option type: %w", err)
			}
			toTy, err := typeexpr.TypeConstraint(o.Type)
			if err != nil {
				return nil, fmt.Errorf("parsing option type: %w", err)
			}
			if exTy != toTy {
				return nil, fmt.Errorf("imported job %q has incompatible option %q: needs type of %v, encountered %v", src.Name, o.Name, exTy.GoString(), toTy.GoString())
			}
		}
	}

	var (
		params []Parameter
		opts   []OptionSpec
	)

	for _, p := range paramMap {
		params = append(params, p)
	}

	for _, o := range optMap {
		opts = append(opts, o)
	}

	dst.Parameters = params
	dst.Options = opts

	return &dst, nil
}
