# Variant 2

This repository contains a development branch of the second major version of [Variant](https://github.com/mumoshu/variant).

See https://github.com/mumoshu/variant for more information on the first version.

Once finished, this repository will eventually take over the `master` branch of the original [variant repository](https://github.com/mumoshu/variant).

# Getting Started

Create an `variant` file that contains the following:

[`examples/getting-started/getting-started.variant`](/examples/getting-started/getting-started.variant):

```hcl
option "namespace" {
  description = "Namespace to interact with"
  type = string
  default = "default"
  short = "n"
}

job "kubectl" {
  parameter "dir" {
    type = string
  }

  exec {
    command = "kubectl"
    args = ["-n", opt.namespace, "-f", param.dir]
  }
}

job "helm" {
  parameter "release" {
    type = string
  }
  parameter "chart" {
    type = string
  }
  option "values" {
    type = list(string)
  }

  exec {
    command = "helm"
    args = ["upgrade", "--install", "-n", opt.namespace, param.release, param.chart]
  }
}

job "deploy" {
  description = "Deploys our application and the infrastructure onto the K8s cluster"

  step "deploy infra" {
    run "helm" {
      release = "app1"
      chart = "app1"
    }
  }

  step "deploy apps" {
    run "kubectl" {
      dir = "deploy/environments/${opt.env}/manifests"
    }
  }
}
```

Now you can run it with `variant`:

`variant run -h` will show you that all the jobs are available via sub-commands:

```console
$ variant run -h
```

```console
Usage:
  variant run [flags]
  variant run [command]

Available Commands:
  deploy
  helm
  kubectl

Flags:
  -h, --help               help for run
  -n, --namespace string   Namespace to interact with

Use "variant run [command] --help" for more information about a command.
```

And `variant run deploy -h` for the usage for the specific job = sub-command named `deploy`:

```
Deploys our application and the infrastructure onto the K8s cluster

Usage:
  variant run deploy [flags]

Flags:
  -h, --help   help for deploy

Global Flags:
  -n, --namespace string   Namespace to interact with
```

As you've seen in the help output, `variant run deploy` runs the `deploy` job, which in turn runs `kubectl` and `helm` to install your apps onto the K8s cluster:

```console
$ variant run deploy
```

Head over to the following per-topic sections for more features:

- [Generating Shims](#generating-shims) to make your Variant command look native
- [Concurrency](#concurrency) section to make `kubectl` and `helm` concurrent so that the installation time becomes minimal

# Features

- **HCL-based DSL**: Terraform-like strongly-typed DSL on top of HCL to define your command. See `Configuration Language` below.
- **Concurrency and Workflow**: Embedded workflow engine with concurrency. See [`Concurrency`](https://github.com/mumoshu/variant2#concurrency) below. Example: [concurrency](https://github.com/mumoshu/variant2/tree/master/examples/concurrency)
- **Configs**: Deep-merging YAML configuration files. Example: [config](https://github.com/mumoshu/variant2/tree/master/examples/config)
- **Secrets**: Deep-merging secret values from Vault, AWS SecretsManager, SOPS, etc. powered by [vals](https://github.com/variantdev/vals). Example: [secret](https://github.com/mumoshu/variant2/tree/master/examples/secret)
- **Testing**: Test framework with `go test`-compatible test runner. Example: [simple](https://github.com/mumoshu/variant2/tree/master/examples/simple)
- **Embeddable**: Easy embedding in any Golang application
- **Easy distribution**: Build a single-executable of your command with Golang
- **Dependency Management**: Dependent files, executable binaries and docker-run shims can be automatically installed and updated with the [variantdev/mod](https://github.com/variantdev/mod) integration. Example: [module](https://github.com/mumoshu/variant2/tree/master/examples/module)

## Generating Shims

If you're distributing this command with your teammates, do use `variant generate shim` to create a shim to make it look like a native command:

```console
$ variant generate shim examples/getting-started/
```

```console
$ cat ./examples/getting-started/getting-started
#!/usr/bin/env variant

import = "."
```

```console
$ ./examples/getting-started/getting-started
Usage:
  getting-started [flags]
  getting-started [command]

Available Commands:
  deploy      Deploys our application and the infrastructure onto the K8s cluster
  helm
  help        Help about any command
  kubectl

Flags:
  -h, --help               help for getting-started
  -n, --namespace string   Namespace to interact with

Use "getting-started [command] --help" for more information about a command.
```

## Concurrency

The example in the [`Getting Started`](#getting-started) guide can be modified by adding `needs` to build a DAG of steps and `concurrency` for setting the desired number of concurrency:

BEFORE:

```hcl
job "deploy" {
  step "deploy infra" {
    run "helm" {
      release = "app1"
      chart = "app1"
    }
  }

  step "deploy apps" {
    run "kubectl" {
      dir = "deploy/environments/${opt.env}/manifests"
    }
  }
}
```

AFTER:

```hcl
job "deploy" {
  concurrency = 2

  step "deploy fluentd" {
    run "helm" {
      release = "fluentd"
      chart = "fluentd"
    }
  }

  step "deploy prometheus" {
    run "helm" {
      release = "prometheus"
      chart = "prometheus"
    }
  }

  step "deploy apps" {
    run "kubectl" {
      dir = "deploy/environments/${opt.env}/manifests"
    }
    needs = ["deploy fluentd", "deploy prometheus"]
  }
}
```

Now, running `variant run deploy` deploys fluend and prometheus concurrently.
Once finished, it deploys your app, as you've declared so in the `needs` attribute of the `run "deploy apps" {}` block.

## Configuration Language

Variant uses its own configuration language based on [the HashiCorp configuration language 2](https://github.com/hashicorp/hcl).

It is designed to allow concise descriptions of your command. The Variant language is declarative, describing an intended goal of the command rather than the steps to compose your command.

In addition to everything available via the [native HCL syntax](https://github.com/hashicorp/hcl/blob/hcl2/hclsyntax/spec.md), Variant language provides the following HCL `blocks` and `functions` to declare your command.

**Blocks**:

- Parameters
- Options
- Jobs
- Tests
- Exec's
- Asserts
- Runs
- Steps

**Functions**:

- All the [Terraform built-in functions](https://www.terraform.io/docs/configuration/functions.html)
- Plus a few Variant-specific functions
  - `jsonpath` ([definition](/pkg/conf/jsonpath.go#L26), [example](/examples/complex/complex.variant#L6))


### Examples

To learn more, see [examples](https://github.com/mumoshu/variant2/tree/master/examples) for working examples covering all these `blocks` and `functions`.

Optionally read the following for overviews on each type of block and functions.

### Blocks

- [Job](#job)
- Parameters
- Options
- Tests
- Exec's
- Asserts
- Runs
- Steps

#### Job

Do only one thing in each "job"

Each job can contain any of the followings, but not two or more of them:

- assert
- exec
- run
- step

This restriction ensures that you can do only one thing in each job, which makes testing and reading the code easier.

That is, a job containing `assert` can be used as a custom assertion "function" and nothing else.

A job containing one or more `step`s can be used as a workflow composed of multiple jobs. Each `step` is restricted to call a single `job`. As each `job` is easily unit testable, this ensures that you can test the workflow without dealing with each job's implementation.

# Learning materials

`hcl`

- https://github.com/apparentlymart/terraform-clean-syntax

`cty`

- https://github.com/zclconf/go-cty
