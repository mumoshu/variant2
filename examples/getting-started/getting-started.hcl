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
