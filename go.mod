module github.com/mumoshu/variant2

go 1.15

require (
	github.com/AlecAivazis/survey/v2 v2.0.5
	github.com/PaesslerAG/jsonpath v0.1.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/fluxcd/pkg/untar v0.0.5
	github.com/fluxcd/source-controller/api v0.2.0
	github.com/go-logr/logr v0.1.0
	github.com/go-playground/universal-translator v0.17.0 // indirect
	github.com/google/go-cmp v0.5.3
	github.com/google/go-github/v27 v27.0.6 // indirect
	github.com/hashicorp/go-multierror v1.0.0
	github.com/hashicorp/hcl/v2 v2.7.0
	github.com/hashicorp/terraform v0.14.0-beta2
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/iancoleman/strcase v0.0.0-20191112232945-16388991a334 // indirect
	github.com/imdario/mergo v0.3.11
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/kr/text v0.1.0
	github.com/leodido/go-urn v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/nlopes/slack v0.6.0
	github.com/pkg/errors v0.9.1
	github.com/rakyll/statik v0.1.7
	github.com/rs/xid v1.2.1
	github.com/spf13/cobra v0.0.5
	github.com/summerwind/whitebox-controller v0.7.1
	github.com/tidwall/gjson v1.3.5
	github.com/twpayne/go-vfs v1.3.6 // indirect
	github.com/ulikunitz/xz v0.5.6 // indirect
	github.com/variantdev/dag v0.0.0-20191028002400-bb0b3c785363
	github.com/variantdev/mod v0.18.0
	github.com/variantdev/vals v0.11.0
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/zclconf/go-cty v1.6.2-0.20201013200640-e5225636c8c2
	github.com/zclconf/go-cty-yaml v1.0.2
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/tools v0.0.0-20201121010211-780cb80bd7fb // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	gopkg.in/go-playground/validator.v9 v9.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	k8s.io/api v0.18.12 // indirect
	k8s.io/apimachinery v0.18.12
	k8s.io/client-go v10.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.6.3
)

replace github.com/summerwind/whitebox-controller v0.7.1 => github.com/mumoshu/whitebox-controller v0.5.1-0.20201028130131-ac7a0743254b

replace k8s.io/client-go v10.0.0+incompatible => k8s.io/client-go v0.18.12
