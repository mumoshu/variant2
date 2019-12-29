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

A job containing one or more `step`s can be used as a workflow composed of multiple jobs. Each `step` is restricted to call a single `job`. As each `job` is easily unit testable, this ensures that you can test the workflow without dealing which each job's implementation.

# Learning materials

`hcl`

- https://github.com/apparentlymart/terraform-clean-syntax

`cty`

- https://github.com/zclconf/go-cty
