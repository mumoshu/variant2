job "test" {
  module = "default"

  exec {
    command = "sh"
    args = ["-c", "helm3 version -c"]
    env = {
      PATH = "${mod.pathaddition}:/bin:/usr/bin:/usr/local/bin"
    }
  }
}

job "ls" {
  module = "default"

  exec {
    command = "sh"
    args = ["-c", "helm3 ls --namespace kube-system"]
    env = {
      PATH = "${mod.pathaddition}:/bin:/usr/bin:/usr/local/bin"
    }
  }
}

job "kubectl-test" {
  module = "default"

  exec {
    command = "sh"
    args = ["-c", "k version --client; k config view --minify"]
    env = {
      PATH = "${mod.pathaddition}:/bin:/usr/bin:/usr/local/bin"
    }
  }
}

job "build" {
  module = "default"

  exec {
    command = "sh"
    args = ["-c", "cat ${context.sourcedir}/Dockerfile"]
  }
}
