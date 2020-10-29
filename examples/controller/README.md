# Running your Variant command as a Kubernetes controller

Running `variant` with a few environment variables allows you to turn your command into a Kubernetes controller.

Let's say your command had a pair of jobs to upsert and destroy the your stack by using e.g. Terraform and/or Helmfile:

- `variant run apply --env prod --ref abcd1234`
- `variant run destroy --env prod --ref abcd1234`

```console
$ cat main.variant
option "env" {
  type = string
}

option "ref" {
  type = string
}

job "apply" {
  exec {
    command = "echo"
    args = ["Deploying ${opt.ref} to ${opt.env}"]
  }
}

job "destroy" {
  exec {
    command = "echo"
    args = ["Destroying ${opt.env}"]
  }
}
```

You can turn this into a Kubernetes controller by setting `<PREFIX>_JOB_ON_APPLY` to the job ran on resource creation or update,
and `<PREFIX>_JOB_ON_DESTROY` to the job ran on resource deletion. The only remaining required envvar is `VARIANT_CONTROLLER_NAME`,
which must be set to whatever name that the controller uses as the name of itself.

As we've seen in the example in the beginning of this section, the on-apply job is `apply` and the on-destroy job is `destroy` so that `variant` invocation
should look like:

```console
$ VARIANT_CONTROLLER_JOB_ON_APPLY=apply \
  VARIANT_CONTROLLER_JOB_ON_DESTROY=destroy \
  VARIANT_CONTROLLER_NAME=resource \
    variant
```

`variant` uses `core.variant.run/v1beta1` `Resource` as the resource to be reconciled by the controller.

That being said, you can let the controller reconcile your resource by creating a `Resource` object with correct arguments -
`env` and `ref` in this example - under the object's `spec` field:

```console
$ kubectl apply -f <(cat EOS
apiVersion: core.variant.run/v1beta1
kind: Resource
metadata:
  name: myresource
spec:
  env: preview
  ref: abc1234
EOS
)
```     

Within a few seconds, the controller will reconcile your `Resource` by running `variant run apply --env preview --ref abc1234`.

You can verify that by tailing controller logs by `kubectl logs`, or browsing the `Reconcilation` object that is created by
the controller to record the reconciliation details:

```console
$ kubectl get reconciliation
NAME           AGE
myresource-2   12m
```

```console
$ kubectl get -o yaml reconciliation myresource-2
apiVersion: core.variant.run/v1beta1
kind: Reconciliation
metadata:
  creationTimestamp: "2020-10-28T12:05:55Z"
  generation: 1
  labels:
    core.variant.run/controller: resource
    core.variant.run/event: apply
    core.variant.run/pod: YOUR_HOST_OR_POD_NAME
  name: myresource-2
  namespace: default
spec:
  combinedLogs:
    data: |
      Deploying abc2345 to preview
  job: apply
  resource:
    env: preview
    ref: abc2345
```

Updating the `Resource` object will result in `variant` running `variant run apply` with the updated arguments:

```console
$ kubectl apply -f <(cat <<EOS
apiVersion: core.variant.run/v1beta1
kind: Resource
metadata:
  name: myresource
spec:
  env: preview
  ref: abc2345
EOS
)
```

```console
$ kubectl get reconciliation
NAME           AGE
myresource-2   12m
myresource-3   12m
```

```cnosole
apiVersion: core.variant.run/v1beta1urce-3
kind: Reconciliation
metadata:
  creationTimestamp: "2020-10-28T12:06:10Z"
  generation: 1
  labels:
    core.variant.run/controller: resource
    core.variant.run/event: apply
    core.variant.run/pod: YOUR_HOST_OR_POD_NAME
  name: myresource-3
  namespace: default
spec:
  combinedLogs:
    data: |
      Deploying abc1234 to preview
  job: apply
  resource:
    env: preview
    ref: abc1234
```

Finally, deleting the `Resource` will let `variant` destroy the underlying resources by running `variant run destroy`
as you've configured:

```console
$ kubectl get reconciliation
NAME           AGE
myresource-2   19m
myresource-3   19m
myresource-4   9s
```

```console
$ kubectl get reconciliation -o yaml myresource-4
apiVersion: core.variant.run/v1beta1
kind: Reconciliation
metadata:
  creationTimestamp: "2020-10-28T12:25:32Z"
  generation: 1
  labels:
    core.variant.run/controller: resource
    core.variant.run/event: apply
    core.variant.run/pod: YOUR_POD_OR_HOST_NAME
  name: myresource-4
  namespace: default
spec:
  combinedLogs:
    data: |
      Destroying preview
  job: destroy
  resource:
    env: preview
    ref: abc1234
```

Now, go build a container image of your Variant command and deploy it as a Kubernetes deployment, and you've finished
deploying your first Variant-powered Kubernetes controller :smile:

We've used simple `echo` as the implementations of `apply` and `destroy` jobs.
But obviously you can run any combinations of tools within your jobs to easily manage whatever "stack".

The list of tools may include:

- Terraform
- AWS CDK
- Kubectl
- Helm
- Helmfile
- Waypoint

## Advanced configuration

- Using non-default CRD

### Using non-default CRD

You can use different apiVersion than the default `core.variant.run/v1beta1` by setting `<PREFIX>_FOR_API_VERSION`,
and different kind than the default `Resource` by setting `<PREFIX>_FOR_KIND`.

For example, to let the controller watch and reconcile `whatever.example.com/v1alpha1` `MyCustomStack` objects, run `variant`
like:

```console
$ VARIANT_CONTROLLER_JOB_ON_APPLY=apply \
    VARIANT_CONTROLLER_JOB_ON_DESTROY=destroy \
    VARIANT_CONTROLLER_NAME=my-custom-stack \
    VARIANT_CONTROLLER_FOR_API_VERSION=whatever.example.com/v1alpha1 \
    VARIANT_CONTROLLER_FOR_KIND=MyCustomStack \
      variant
```
