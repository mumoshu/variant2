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
      condition = event.type == "exec_start" && event.exec_start.step == ""
      format = jsonencode(event.exec_start)
    }

    collect {
      condition = event.type == "exec_end"
      format = jsonencode(event.exec_end)
    }

    collect {
      condition = event.type == "run_start"
      format = jsonencode(event.run_start)
    }

    forward {
      run "save-logs" {
        logfile = log.file
      }
    }
  }
}
