option namespace {
  description = "Namespace to interact with"
  type = string
  default = "default"
  short = "n"
}

job "app deploy" {
  script = <<EOS
    kubectl -n ${param.namespace} apply -f ${context.sourcedir}/manifests/
EOS
}
