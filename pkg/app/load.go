package app

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/ext/userfunc"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/variantdev/mod/pkg/depresolver"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/mumoshu/variant2/pkg/conf"
	fs2 "github.com/mumoshu/variant2/pkg/fs"
)

const (
	DefaultCacheDir = ".variant2/cache"
)

type hcl2Loader struct {
	Parser *hclparse.Parser
}

type configurable struct {
	Body hcl.Body
}

func loadFiles(fs *fs2.FileSystem, filenames ...string) (map[string][]byte, error) {
	srcs := map[string][]byte{}

	for _, filename := range filenames {
		src, err := fs.ReadFile(filename)
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

func (t *configurable) HCL2Config() (*HCL2Config, map[string]function.Function, error) {
	config := &HCL2Config{}

	funcs := conf.Functions(".")

	ctx := &hcl.EvalContext{
		Functions: funcs,
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

	userfuncs, remain, diags := userfunc.DecodeUserFunctions(t.Body, "function", func() *hcl.EvalContext {
		return ctx
	})
	if diags.HasErrors() {
		return config, nil, diags
	}

	for k, v := range userfuncs {
		if _, duplicated := funcs[k]; duplicated {
			return nil, nil, fmt.Errorf("function %q can not be overridden by a user function with the same name", k)
		}

		funcs[k] = v
	}

	diags = gohcl.DecodeBody(remain, ctx, config)
	if diags.HasErrors() {
		return config, nil, diags
	}

	return config, funcs, nil
}

type Instance struct {
	Sources map[string][]byte
	Dir     string
}

type Setup func(*Options) (*Instance, error)

func FromFile(path string) Setup {
	return func(_ *Options) (*Instance, error) {
		fs := &fs2.FileSystem{}

		srcs, err := loadFiles(fs, path)
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
	return func(options *Options) (*Instance, error) {
		fs := &fs2.FileSystem{}

		files, err := findVariantFiles(fs, options.CacheDir, dir)
		if err != nil {
			return nil, err
		}

		srcs, err := loadFiles(fs, files...)
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
	return func(_ *Options) (*Instance, error) {
		return &Instance{
			Sources: srcs,
			Dir:     "",
		}, nil
	}
}

type Options struct {
	CacheDir string
}

type Option func(options *Options)

func WithCacheDir(dir string) Option {
	return func(options *Options) {
		options.CacheDir = dir
	}
}

func New(setup Setup, opts ...Option) (*App, error) {
	var options Options

	for _, o := range opts {
		o(&options)
	}

	instance, err := setup(&options)
	if err != nil {
		return nil, err
	}

	nameToFiles, cc, funcs, err := newConfigFromSources(instance.Sources)

	app := &App{
		Files: nameToFiles,
		Trace: os.Getenv("VARIANT_TRACE"),
		Funcs: funcs,
	}

	if err != nil {
		return app, err
	}

	return newApp(app, cc, NewImportFunc(instance.Dir, func(path string) (*App, error) {
		return New(FromDir(path), opts...)
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

func findVariantFiles(fs *fs2.FileSystem, cacheDir string, dirPathOrURL string) ([]string, error) {
	var dir string

	s := strings.Split(dirPathOrURL, "::")

	//nolint:nestif
	if len(s) > 1 {
		forcePrefix := s[0]

		u, err := url.Parse(s[1])
		if err != nil {
			return nil, err
		}

		if cacheDir == "" {
			cacheDir = DefaultCacheDir
		}

		remote, err := depresolver.New(depresolver.Home(cacheDir))
		if err != nil {
			return nil, err
		}

		remote.DirExists = func(path string) bool {
			info, _ := fs.Stat(path)
			if info != nil && info.IsDir() {
				return true
			}

			return false
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

	files, err := conf.FindVariantFiles(fs, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s files: %w", conf.VariantFileExt, err)
	}

	return files, nil
}

func newConfigFromSources(srcs map[string][]byte) (map[string]*hcl.File, *HCL2Config, map[string]function.Function, error) {
	l := &hcl2Loader{
		Parser: hclparse.NewParser(),
	}

	c, nameToFiles, err := l.loadSources(srcs)
	if err != nil {
		return nameToFiles, nil, nil, err
	}

	cc, funcs, err := c.HCL2Config()

	return nameToFiles, cc, funcs, err
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

		//nolint:nestif
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
