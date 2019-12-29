option namespace {
  description = "Namespace to interact with"
  type = string
  default = "default"
  short = "n"
}

job "shell" {
  parameter "script" {
    type = string
  }

  parameter "path" {
    type = string
  }

  exec {
    command = "bash"
    args = ["-c", param.script]
    env = {
      PATH = param.path
    }
  }
}

job "app deploy" {
  option "path" {
    type = string
    default = ".:/bin:/usr/bin:${abspath("${context.sourcedir}/mocks/kubectl")}"
  }
  run "shell" {
    script = <<EOS
    kubectl -n ${opt.namespace} apply -f ${context.sourcedir}/manifests/
EOS
    path = opt.path
  }

  assert "path" {
    condition = opt.path != ""
  }
}
