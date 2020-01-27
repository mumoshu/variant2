job "echo" {
  parameter "message" {
    type = string
  }

  exec {
    command = "echo"
    args = ["text=${param.message}"]
  }
}

job "save-logs" {
  parameter "logfile" {
    type = string
  }

  exec {
    command = "cat"
    args = [param.logfile]
  }
}

job "test" {
  run "echo" {
    message = "foo"
  }

  log {
    collect {
      condition = event.type == "exec"
      format = "exec=${jsonencode(event.exec)}"
    }

    collect {
      condition = event.type == "run"
      format = "run=${jsonencode(event.run)}"
    }

    file = "${context.sourcedir}/log.txt"

    forward {
      run "save-logs" {
        logfile = log.file
      }
    }
  }
}
