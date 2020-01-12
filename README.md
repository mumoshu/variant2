# Variant 2

This repository contains a development branch of the second major version of [Variant](https://github.com/mumoshu/variant).

See https://github.com/mumoshu/variant for more information on the first version.

Once finished, this repository will eventually take over the `master` branch of the original [variant repository](https://github.com/mumoshu/variant).

# Getting Started

Create an `hcl` file that contains the following:

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

As you've seen in the help output, `variant run deploy` runs the `deploy` job, which in turn runs `kubectl` and `helm` to install your apps onto the K8s cluster:

```console
$ variant run deploy
```

# Features

- Terraform-like strongly-typed DSL on top of HCL to define your command (See `Configuration Language` below)
- Deep-merging YAML configuration files
- Deep-merging secret values from Vault, AWS SecretsManager, SOPS, etc. powered by [vals](https://github.com/variantdev/vals)
- Test framework with `go test`-compatible test runner
- Easy embedding in any Golang application
- Build a single-executable of your command with Golang

# Configuration Language

Variant uses its own configuration language based on [the HashiCorp configuration language 2](https://github.com/hashicorp/hcl).

It is designed to allow concise descriptions of your command. The Variant language is declarative, describing an intended goal of the command rather than the steps to compose your command.

In addition to everything available via the [native HCL syntax](https://github.com/hashicorp/hcl/blob/hcl2/hclsyntax/spec.md), Variant language provides the following HCL `blocks` to declare your command:

- Parameters
- Options
- Jobs
- Tests
- Exec's
- Asserts
- Runs
- Steps

## Job

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
