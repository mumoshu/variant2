# Variant 2

> This repository contains a development branch of the second major version of [Variant](https://github.com/mumoshu/variant).
> 
> See https://github.com/mumoshu/variant for more information on the first version.
> 
> Once finished, this repository will eventually take over the `master` branch of the original [variant repository](https://github.com/mumoshu/variant).

# Features

- **HCL-based DSL**: Terraform-like strongly-typed DSL on top of HCL to define your command. See `Configuration Language` below.
- **Concurrency and Workflow**: Embedded workflow engine with concurrency. See [`Concurrency`](https://github.com/mumoshu/variant2#concurrency) below. Example: [concurrency](https://github.com/mumoshu/variant2/tree/master/examples/concurrency)
- **Auto-prompt**: Variant prompts for missing arguments to your command so that your user is only needed to provide missing arguments on the fly, instead of rerunning the whole command by repeating every argument.
- **Configs**: Deep-merging YAML configuration files. Example: [config](https://github.com/mumoshu/variant2/tree/master/examples/config)
- **Secrets**: Deep-merging secret values from Vault, AWS SecretsManager, SOPS, etc. powered by [vals](https://github.com/variantdev/vals). Example: [secret](https://github.com/mumoshu/variant2/tree/master/examples/secret)
- **Testing**: Test framework with `go test`-compatible test runner. Example: [simple](https://github.com/mumoshu/variant2/tree/master/examples/simple)
- **Embeddable**: Easy embedding in any Golang application
- **Easy distribution**: Build a single-executable of your command with Golang
- **Dependency Management**: Dependent files, executable binaries and docker-run shims can be automatically installed and updated with the [variantdev/mod](https://github.com/variantdev/mod) integration. Example: [module](https://github.com/mumoshu/variant2/tree/master/examples/module)
- **Run as a Kubernetes controller**: You can easily turn your Variant command into a Kubernetes controller. See [examples/controller](examples/controller)
- **Integrations**: Integrates nicely with Slack, GitHub. You can run Variant command in response to Slack message, GitHub issue comment, commit push, etc.

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

Once you're finished developing the command, let's build a single executable binary of the command for easy distribution:

```console
$ variant export binary ./ build/myapp
```

The exported executable binary accepts the same arguments as `variant run`:

```console
$ ./build/myapp -h

$ ./build/myapp run deploy
```

Congratulations! You're now ready to dive deep and solve your own problems with Variant.

Still curious how Variant helps developing your own command as it grows?

Head over to the following per-topic sections for more features:

- [Writing Commands](#writing-commands) to learn the Variant DSL for writing commands
- [Debugging Commands](#debugging-commands) to learn how to debug your Variant command
- [Writing Tests](#writing-tests) to learn the Variant DSL for writing tests
- [Generating Shims](#generating-shims) to make your Variant command look native
- [Compiling Command](#compiling-command) to export Go source or an executable binary
- [Running Command From Other Directory](#running-command-from-another-directory)
- [Concurrency](#concurrency) section to make `kubectl` and `helm` concurrent so that the installation time becomes minimal
- [Log Collection](#log-collection) to filter and forward log of commands and the arguments passed to them along with their outputs
- Use [Split, Merge and Import](#split-merge-and-import) to split, compose and tidy Variant commands
- [JSON Configuration Syntax](#json-configuration-syntax) can be used as an alternative to HCL2-based one
- [Slack integration](#slack-integration) to turn your command into a Slack bot

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

## Compiling Command

As we've covered in the [Getting Started](#getting-started) guide, `variant export` sub-commands can be used to export
your command in various formats.

`variant export binary SRC CMD` generates an executable binary at the path `CMD` from your command defined under the
directory `SRC`:

```console
$ variant export binary ./ build/myapp
```

The exported executable binary accepts the same arguments as `variant run`.
So `myapp -h` corresponds to `cd $SRC; variant run -h`, while
`myapp run deploy` corresponds to `cd $SRC; variant run deploy`.

`variant export go SRC/CMD PKG` generates a directory at `PKG/CMD` that contains Go source files that can be built by
running `go build PKG/CMD`.

Assuming you already have `go` installed, you can run `variint export go src/myapp build`, then edit code under
`build/myapp` to make any customization that can't be done with [shims](#generating-shims), and finally build
an executable with `go build -o myapp ./build/myapp`.

## Running Command From Other Directory

Usually, when your command has been defined under the directory `path/to/your/command`, `variant run` requires you to `chdir` to
it before running.

To be clear, `cd $SRC; variant run` can be used to run it from another directory. However with that you command cannot
access the actual current directory, as you've already `cd`ed.

You can also use a [shim](#generating-shims) or an [exported binary](#compiling-command) to make it runnable from any
directory. But it takes some time so probably you'd like to run it from any directory without a `variant export` step
while developing?

The `VARIANT_DIR` environment variable might be the solution. When `variant` recognizes it, it reads the command from the directory
specified by it.

Just run:

```console
VARIANT_DIR=$SRC variant run
```

Your command can now be run without `cd` and still has access to the current directory.

## Split, Merge and Import

Do you have a huge `yourcmd.variant` that needs to be split for readability?

`path/to/yourcmd.variant`:

```hcl
job "foo" {
  # snip
}

job "bar" {
  # snip
}

job "baz" {
  # snip
}
```

Variant usually works per-directory basis. That is, it loads and merges all the `.variant` files in a directory to form a single command.

That is, you can just split the file into three `.variant` files in the same directory to split the huge file:

`path/to/yourcmd_foo.variant`:
```hcl
job "foo" {
  parameter "param1" {
    # snip
  }

  # snip
}
```

`path/to/yourcmd_bar.variant`:
```hcl
job "bar" {
  # snip
}
```

`path/to/yourcmd_baz.variant`:
```hcl
job "baz" {
  # snip
}
```

Okay that works. But you ended up too many files in a single directory?

A "parent" variant file containing `import` or `imports` can be used to load all the `*.variant` files in the directory into the current `job`.

`path/to/yourcmd.variant`:
```hcl
job "foo" {
  import = "./foo"
}

job "bar" {
  import = "./bar"
}
```

`path/to/foo/foo.variant`:
```hcl
parameter "param1" {
  # snip
}

# snip
```

Note that `imports` is the newer variant of `import` that supports multiple sources to be imported.

Also, you can import following sources:

- Relative path to local directory (A local path that doesn't start with `/`, like `foo/bar`)
- Absolute path to local directory (An absolute path that starts with `/`, like `/variant/modules/example.com/foo/bar`)
- URLs to Git repository (`REPO_URL@PATH/TO/DIR?ref=BRANCH`, e.g. `git::ssh://git@github.com/mumoshu/variant2@examples/advanced/import/foo?ref=master`)

See the [import](/examples/advanced/import) example for the full declaration of this command for reference.

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

## Log Collection

`log` block(s) placed under a `job` can be used to forward log of commands and the arguments passed to them along with their outputs.

Let's assume you'd like to collect log for the `test` job:

```hcl
job "test" {
  run "echo" {
    message = "foo"
  }
}
```

Collecting all the `run` and `exec` that are executed as part of this `job` along with all the metadata attached to respective event can be achieved by filtering events by `condition` and formatting log messages with `format` like:

```hcl
log {
  collect {
    condition = event.type == "exec"
    format = "exec=${jsonencode(event.exec)}"
  }

  collect {
    condition = event.type == "run"
    format = "run=${jsonencode(event.run)}"
  }

  # The log file to be created
  file = "${context.sourcedir}/log.txt"
}
```

In case you need to post-process or upload the log file, use `forward` to pass the log file to any Variant `job`:

```hcl
file = "${context.sourcedir}/log.txt"

forward {
  run "save-logs" {
    logfile = log.file
  }
}

# ...

job "save-logs" {
  option "logfile" {
    type = string
  }

  exec {
     command = "upload-to-jira"
     args = ["-f", opt.logfile]
  }
}
```

See the [logcollection](/examples/advanced/logcollection) example for the full declaration of this command for reference.

## Slack integration

Slack integration turns your Variant command into a Slack bot.

The bot is installed onto a specific Slack channel as a Slack app to listen any message to trigger a command.

Once you run:

```
SLACK_BOT_TOKEN=... SLACK_VERIFICATION_TOKEN=... variant start slackbot -n mycmd
```

in a directory that has a valid Variant command, it starts a RTM session to listen messages posted to the Slack channel.

When a message starting with:

```
/mycmd
```

is posted to the channel, the bot inteprets

```
/mycmd <CMD> <FLAGS>
```

as:

```
variant run <CMD> <FLAGS>
```

and runs it.

**Auto-prompting via interactive messages**

One of cool features of the bot is that when you missed to specify values for certain options, it will automatically start a interactive session to let you select and input missing values within Slack. You don't need to remember all the flags nor repeat lengthy commands anymore.

## Writing Commands

Variant uses its own configuration language based on [the HashiCorp configuration language 2](https://github.com/hashicorp/hcl).

It is designed to allow concise descriptions of your command. The Variant language is declarative, describing an intended goal of the command rather than the steps to compose your command.

In addition to everything available via the [native HCL syntax](https://github.com/hashicorp/hcl/blob/hcl2/hclsyntax/spec.md), Variant language provides the following HCL `blocks` and `functions` to declare your command.

- [Blocks](#blocks)
- [Functions](#functions)
- [Examples](#examples)

### Blocks

- [Job](#job)
- [Parameter](#parameter)
- [Option](#option)
- [Run](#run)
  - [Indirect Run](#indirect-run)
- [Exec](#exec)
- [Config](#config)
- [Test](#test)
- Steps
- Asserts

#### job

`job "CMD SUBCMD {}` is a job that can be run via `run "CMD SUBCMD" {}` or `variant run CMD SUBCMD`

Do only one thing in each "job"

Each job can contain any of the followings, but not two or more of them:

- assert
- exec
- run
- step

This restriction ensures that you can do only one thing in each job, which makes testing and reading the code easier.

That is, a job containing `assert` can be used as a custom assertion "function" and nothing else.

A job containing one or more `step`s can be used as a workflow composed of multiple jobs. Each `step` is restricted to call a single `job`. As each `job` is easily unit testable, this ensures that you can test the workflow without dealing with each job's implementation.

Options:

`job` has the following attributes:

- `private`: when set to `true` by writing `private = true`, the job is hidden from the command-line help.

#### parameter

`parameter "NAME" {}` is Nth positional argument to `job` that can be pased via `run "the job" { NAME = "val1" }` or `variant run the job val1`

#### option

`option "NAME" {}` is a named argument to `job` that can be passed via `run "the job" { NAME = "val1" }` or `varuant run the job --NAME val1`

#### config

`config "NAME" {}` is a layered configuration named `NAME`

Let's assume `defaults.yaml` has the following content:

```yaml
api:
  endpoint: api.example.com
replicas: 2
```

And a Variant command that contains a job named `thing` that has a `config` block named `conf`:

```hcl
job "thing" {
  option "env" {
    type = string
  }

  config "myconf" {
    source file {
      path = "defaults.yaml"
    }

    source job {
      name = "values"
      args = {
        env = opt.env
      }
    }

    source job {
      name = "value"
      args = {
        
      }
      key = "key1"
      format = "text"
    }
  }

  exec {
    command = "echo"
    args = list("apiendpoint=${conf.myconf.api.endpoint}, replicas=${conf.myconf.replicas}, env=${conf.myconf.env}, key1=${conf.myconf.key1}")
  }
}

job "values" {
  option "env" {
    type = string
  }

  exec {
    command = "echo"
    args = list("env: ${opt.env}")
  }
}

job "value" {
  exec {
    command = "echo"
    args = list("val1")
  }
}
```

The first source loads a file at `defaults.yaml` and merges the keys and values in it into `conf`.

Similarly, the second source loads keys and values from a job named `values` and merges them into `conf`.

The third sets the value for the key `key1` where the value is the output of a job named `value`.

The configuration `myconf` can be referenced by `${conf.myconf.KEY}` as shown in the `exec` block within the `thing` job above.

Let's run `variant run thing --env prod` and the output should be:

```console
apiendpoint=api.example.com, replicas=2, env=prod, key1=val1
```

#### run

`run` runs a job with args. `run` is available within `job` and `test`.

```hcl
job "a job" {
  run "another job" {
    param1 = "val1"
    opt1 = "val2"
  }
}

job "another job" {
  parameter "param1" {
    type = string
  }
  option "opt1" {
    type = string
  }

  exec {
    // ...
  }
}
```

#### Indirect Run

Sometimes you'd encounter a situation that you need indirection.

That is, the job name must be dynamically determined depending on a variable, parameter, or an option.

Variant2 provides an alternative `run` block syntax for that, which basically omits the NAME label in the `run "NAME" { }` you've previously seen:

To call the `example` job with the parameter `foo` set to `"FOO"` and the option `bar` set to `"BAR"`, you write the `run` block like the below:

```hcl
run {
  job = "example"

  with = {
    foo = "FOO"
    bar = "BAR"
  }
}
```

#### step

TODO

#### exec

An `exec` block executes the OS command.

Available attributes:

- `cmd`: The path to the executable binary/script
- `args`: The arguments to be passed to the command
- `env`: The environment variables given to the command
- `dir`: The working directory

### Functions

- All the [Terraform built-in functions](https://www.terraform.io/docs/configuration/functions.html)
- Plus a few Variant-specific functions
  - `jsonpath` ([definition](/pkg/conf/jsonpath.go#L26), [example](/examples/complex/complex.variant#L6))

### Examples

To learn more, see [examples](https://github.com/mumoshu/variant2/tree/master/examples) for working examples covering all these `blocks` and `functions`.

Optionally read the following for overviews on each type of block and functions.

### JSON Configuration Syntax

`Variant` has a JSON-based dialect of its DSL. It's based on the HCL's build-in feature to natively support JSON, so the conversion rules between HCL and JSON is very similar to famous HCL-based language like [Terraform](https://www.terraform.io/docs/configuration/syntax-json.html).

Generally speaking, you can use blocks, attributes and expressions with a little overhead. See the [options-json](/examples/options-json) example for more details.

## Debugging Commands

Setting the environment variable `VARIANT_TRACE` to a non-empty value enables the trace logging.

It might be handy when you are curious how a third-party Variant command works, or when it seems like your command has an unknown bug that must be fixed.

Example:

```console
$ VARIANT_TRACE=1 VARIANT_DIR=examples/issues/8-logging ./variant run example
go build -o variant ./pkg/cmd
foobar
TRACE   {"Type":"exec","Time":"2020-04-09T16:01:37.436145+09:00","Run":null,"Exec":{"Command":"echo","Args":["foobar"]}}exec={"args":["foobar"],"command":"echo"}
```

## Writing Tests

`Variant` has its own testing framework composed of the test runner and the config syntax.

The test runner can be triggered by executing `variant test`. Upon run, the runner searches for test files whose names suffixed with `_test.variant` , parses it and run all the tests defined within it.
 
The test configuration syntax is as simple as declaring `test "CMD1 SUBCMD1 {}`, which is a set of unit tests for `job "CMD1 SUBCMD1"`.

In each `test`, you can use `case`, `run` and `assert` blocks to define tests.

`case "case1" { varname = value }` defines a test case named `case1` with the test data consists of a single variable called `varname` whose value is `value`. You can include one or more variables in a `case`.

`run "jobname" { arg1 = value }` specifies which `job` is run and which args is passed to the job run. This syntax is equivalent to `run` under `job`s and `step`s.

Each `assert { condition = expression }` block adds a assertion that is run by the test framework. When the expression contained in the `condition` evaluated to `false`, the test fails.

Example:
   
```hcl
test "options" {
  case "ok1" {
    exitstatus = 0
    err = ""
    out = trimspace(<<EOS
1 2 3 a b|c
EOS
    )
  }

  run "test" {
    int1 = 1
    ints1 = list(2,3)
    str1 = "a"
    strs1 = list("b","c")
  }

  assert "error" {
    condition = run.err == case.err
  }

  assert "out" {
    condition = (run.res.set && run.res.stdout == case.out) || !run.res.set
  }

  assert "exitstatus" {
    condition = run.res.exitstatus == case.exitstatus
  }
}
```

For more examples, please browse `examples` directories and look for any files ending with `_test.variant`.

# Learning materials

The `Variant` DSL is very much based on HCL and CTY.

The following is a set of recommended reading for learning HCL and CTY.

`hcl`

- https://github.com/apparentlymart/terraform-clean-syntax

`cty`

- https://github.com/zclconf/go-cty
