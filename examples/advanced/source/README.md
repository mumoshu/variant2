# Flux "Source Controller" Integration

`variant2` integrates with [source-controller](https://github.com/fluxcd/source-controller) to let the controller fetches "sources" like Git repositories, S3 buckets, and so on for your Variant command.

This integration allows you to run your Variant command in e.g. your CI pipeline while source-controller acts as a "cache" for the latest observed revision of your Git repository. This is much faster than your Variant command running `git clone` from scrach on each run, or much easier than implementing Git repository cache on your own.

To give it a try locally, you can follow the below steps:

- Run a `kind` cluster
- Install source-controller CRDs with `kubectl apply -f crds/`
- Install the sample `source` resource with `kubectl apply -f source/myrepo.yaml`
- Run `variant run example`

`variant run example` fetches the latest observed revision of the Git repository configured in the `myrepo` "source" resource,
by interacting with source-controller, and runs a series of commands on the fetched content. 

## Notes

- See [the API reference](https://github.com/fluxcd/source-controller/blob/main/docs/api/source.md#source-api-reference) for more source "kinds" like Bucket, HelmChart, HelmRepository, etc.
- Run `make source-controller-crds` to update the source-controller CRDs.
