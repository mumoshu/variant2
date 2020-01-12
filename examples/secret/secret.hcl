option "app" {
  type = string
}

option "env" {
  type = string
}

job "getfoo" {
  parameter "param1" {
    type = string
  }
  exec {
    command = "echo"
    args = [
      "getfoo: ref+echo://${opt.app},${opt.env},${param.param1}"
    ]
  }
}

job "secret view" {
  // sec.foo
  secret "foo" {
    source file {
      path = "${context.sourcedir}/sec/defaults.yaml"
      default = ""
    }

    source file {
      path = "${context.sourcedir}/sec/${opt.env}.yaml"
    }

    source file {
      path = "${context.sourcedir}/sec/${opt.app}.yaml"
    }

    source file {
      path = "${context.sourcedir}/sec/${opt.app}.${opt.env}.yaml"
      default = ""
    }

    source job {
      name = "getfoo"
      args = {
        app = opt.app
        env = opt.env
        param1 = "PARAM1"
      }
      format = "yaml"
    }
  }

  exec {
    command = "echo"
    args = [
      jsonencode(sec.foo)
    ]
  }
}
