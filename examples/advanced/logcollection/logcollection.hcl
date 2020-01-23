job "echo" {
  parameter "message" {
    type = string
  }

  exec {
    command = "echo"
    args = ["text=", param.message]
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
      condition = event.type == "exec" && event.exec.step == ""
      format = jsonencode(event.exec)
    }

    collect {
      condition = event.type == "run"
      format = jsonencode(event.run)
    }

    forward {
      run "save-logs" {
        logfile = log.file
      }
    }
  }
}
