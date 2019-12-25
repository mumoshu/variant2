option namespace {
  description = "Namespace to interact with"
  type = string
  default = "default"
  short = "n"
}

job "app deploy" {
  step {
    script = <<EOS
    kubectl -n ${opt.namespace} apply -f ${context.sourcedir}/manifests/
EOS
  }
}
