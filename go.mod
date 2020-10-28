module github.com/mumoshu/variant2

go 1.15

require (
	github.com/AlecAivazis/survey/v2 v2.0.5
	github.com/PaesslerAG/jsonpath v0.1.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/go-playground/universal-translator v0.17.0 // indirect
	github.com/google/go-cmp v0.4.0
	github.com/google/go-github/v27 v27.0.6 // indirect
	github.com/hashicorp/go-multierror v1.0.0
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl/v2 v2.3.0
	github.com/hashicorp/terraform v0.12.18
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
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/summerwind/whitebox-controller v0.7.1
	github.com/tidwall/gjson v1.3.5
	github.com/twpayne/go-vfs v1.3.6 // indirect
	github.com/ulikunitz/xz v0.5.6 // indirect
	github.com/urfave/cli v1.22.1 // indirect
	github.com/variantdev/dag v0.0.0-20191028002400-bb0b3c785363
	github.com/variantdev/mod v0.18.0
	github.com/variantdev/vals v0.4.0
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/zclconf/go-cty v1.2.1
	github.com/zclconf/go-cty-yaml v1.0.1
	golang.org/x/crypto v0.0.0-20200214034016-1d94cc7ab1c6 // indirect
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a
	golang.org/x/tools v0.0.0-20200331025713-a30bf2db82d4 // indirect
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
	gopkg.in/go-playground/validator.v9 v9.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c
	k8s.io/api v0.0.0-20190918155943-95b840bb6a1f // indirect
	k8s.io/apiextensions-apiserver v0.0.0-20190918161926-8f644eb6e783 // indirect
	k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/klog v1.0.0 // indirect
	sigs.k8s.io/controller-runtime v0.4.0
)

replace (
	k8s.io/api v0.0.0-20190918155943-95b840bb6a1f => k8s.io/api v0.0.0-20190918155943-95b840bb6a1f
	k8s.io/apiextensions-apiserver v0.0.0-20190918161926-8f644eb6e783 => k8s.io/apiextensions-apiserver v0.0.0-20190918161926-8f644eb6e783
	k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655 => k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible => k8s.io/client-go v0.0.0-20190918160344-1fbdaa4c8d90
)
