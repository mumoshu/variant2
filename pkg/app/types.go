package app

import (
	"io"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/mumoshu/variant2/pkg/source"
)

type Config struct {
	Name string `hcl:"name,label"`

	Sources []ConfigSource `hcl:"source,block"`
}

type ConfigSource struct {
	Type string `hcl:"type,label"`

	Body hcl.Body `hcl:",remain"`
}

type SourceFile struct {
	Path    *string  `hcl:"path,attr"`
	Paths   []string `hcl:"paths,optional"`
	Default *string  `hcl:"default,attr"`
	Key     *string  `hcl:"key,attr"`
}

type Step struct {
	Name string `hcl:"name,label"`

	Run StaticRun `hcl:"run,block"`

	Needs *[]string `hcl:"need,attr"`
}

type Exec struct {
	Command hcl.Expression `hcl:"command,attr"`

	Args hcl.Expression `hcl:"args,attr"`
	Env  hcl.Expression `hcl:"env,attr"`
	Dir  hcl.Expression `hcl:"dir,attr"`

	Interactive *bool `hcl:"interactive,attr"`
}

type DependsOn struct {
	Name string `hcl:"name,label"`

	Items hcl.Expression `hcl:"items,attr"`
	Args  hcl.Expression `hcl:"args,attr"`
}

type LazyStaticRun struct {
	Run []StaticRun `hcl:"run,block"`
}

type StaticRun struct {
	Name string `hcl:"name,label"`

	Args map[string]hcl.Expression `hcl:",remain"`
}

type LazyDynamicRun struct {
	Run []DynamicRun `hcl:"run,block"`
}

type DynamicRun struct {
	Job       string         `hcl:"job,attr"`
	Args      hcl.Expression `hcl:"with,attr"`
	Condition hcl.Expression `hcl:"condition,attr"`
}

type Parameter struct {
	Name string `hcl:"name,label"`

	Type    hcl.Expression `hcl:"type,attr"`
	Default hcl.Expression `hcl:"default,attr"`
	Envs    []EnvSource    `hcl:"env,block"`

	Description *string `hcl:"description,attr"`
}

type EnvSource struct {
	Name string `hcl:"name,label"`
}

type SourceJob struct {
	Name string `hcl:"name,attr"`
	// This results in "no cty.Type for hcl.Expression" error
	// Arguments map[string]hcl2.Expression `hcl:"args,attr"`
	Args   hcl.Expression `hcl:"args,attr"`
	Format *string        `hcl:"format,attr"`
	Key    *string        `hcl:"key,attr"`
}

type OptionSpec struct {
	Name string `hcl:"name,label"`

	Type        hcl.Expression `hcl:"type,attr"`
	Default     hcl.Expression `hcl:"default,attr"`
	Description *string        `hcl:"description,attr"`
	Short       *string        `hcl:"short,attr"`
}

type Variable struct {
	Name string `hcl:"name,label"`

	Type  hcl.Expression `hcl:"type,attr"`
	Value hcl.Expression `hcl:"value,attr"`
}

type JobSpec struct {
	// Type string `hcl:"type,label"`
	Name string `hcl:"name,label"`

	Version *string `hcl:"version,attr"`

	Module hcl.Expression `hcl:"module,attr"`

	Description *string      `hcl:"description,attr"`
	Parameters  []Parameter  `hcl:"parameter,block"`
	Options     []OptionSpec `hcl:"option,block"`
	Configs     []Config     `hcl:"config,block"`
	Secrets     []Config     `hcl:"secret,block"`
	Variables   []Variable   `hcl:"variable,block"`

	Concurrency hcl.Expression `hcl:"concurrency,attr"`

	SourceLocator hcl.Expression `hcl:"__source_locator,attr"`

	Deps    []DependsOn    `hcl:"depends_on,block"`
	Exec    *Exec          `hcl:"exec,block"`
	Assert  []Assert       `hcl:"assert,block"`
	Fail    hcl.Expression `hcl:"fail,attr"`
	Import  *string        `hcl:"import,attr"`
	Imports *[]string      `hcl:"imports,attr"`

	// Private hides the job from `variant run -h` when set to true
	Private *bool `hcl:"private,attr"`

	Log *LogSpec `hcl:"log,block"`

	Steps []Step `hcl:"step,block"`

	Body hcl.Body `hcl:",remain"`

	Sources []Source `hcl:"source,block"`
}

type Source struct {
	ID string `hcl:"id,label"`
	// Kind is the kind of source-controller source kind like "GitRepository" and "Bucket"
	Kind string `hcl:"kind,attr"`
	// Namespace is the K8s namespace of the source-controller source
	Namepsace string `hcl:"namespace,attr"`
	// Name defaults to ID
	Name string `hcl:"name,attr"`
}

type LogSpec struct {
	File     hcl.Expression `hcl:"file,attr"`
	Stream   hcl.Expression `hcl:"stream,attr"`
	Collects []Collect      `hcl:"collect,block"`
	Forwards []Forward      `hcl:"forward,block"`
}

type Collect struct {
	Condition hcl.Expression `hcl:"condition,attr"`
	Format    hcl.Expression `hcl:"format,attr"`
}

type Forward struct {
	Run *StaticRun `hcl:"run,block"`
}

type Assert struct {
	Name string `hcl:"name,label"`

	Condition hcl.Expression `hcl:"condition,attr"`
}

type HCL2Config struct {
	Jobs    []JobSpec `hcl:"job,block"`
	Tests   []Test    `hcl:"test,block"`
	JobSpec `hcl:",remain"`
}

type Test struct {
	Name string `hcl:"name,label"`

	Variables []Variable `hcl:"variable,block"`
	Cases     []Case     `hcl:"case,block"`
	Run       StaticRun  `hcl:"run,block"`
	Assert    []Assert   `hcl:"assert,block"`

	SourceLocator hcl.Expression `hcl:"__source_locator,attr"`

	ExpectedExecs []Expect `hcl:"expect,block"`
}

type Expect struct {
	// Type must be `exec` for now
	Type string `hcl:"type,label"`

	Command hcl.Expression `hcl:"command,attr"`
	Args    hcl.Expression `hcl:"args,attr"`
	Dir     hcl.Expression `hcl:"dir,attr"`
}

type expectedExec struct {
	Command string
	Args    []string
	Dir     string
}

type Case struct {
	SourceLocator hcl.Expression `hcl:"__source_locator,attr"`

	Name string `hcl:"name,label"`

	Args map[string]hcl.Expression `hcl:",remain"`
}

type App struct {
	BinName string

	Files     map[string]*hcl.File
	Config    *HCL2Config
	JobByName map[string]JobSpec

	Stdout, Stderr io.Writer

	Trace string

	sourceClient *source.Client

	initMu sync.Mutex

	Funcs map[string]function.Function
}
