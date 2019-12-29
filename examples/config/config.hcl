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
      "getfoo: ${opt.app},${opt.env},${param.param1}"
    ]
  }
}

job "config view" {
  // conf.foo
  config "foo" {
    source file {
      path = "${context.sourcedir}/conf/defaults.yaml"
      default = ""
    }

    source file {
      path = "${context.sourcedir}/conf/${opt.env}.yaml"
    }

    source file {
      path = "${context.sourcedir}/conf/${opt.app}.yaml"
    }

    source file {
      path = "${context.sourcedir}/conf/${opt.app}.${opt.env}.yaml"
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
      jsonencode(conf.foo)
    ]
  }
}
