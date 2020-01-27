job "one" {
  option "delay" {
    type = number
  }
  exec {
    command = "bash"
    args = ["-c", "sleep ${opt.delay}; echo 1"]
  }
}

job "two" {
  option "delay" {
    type = number
  }
  exec {
    command = "bash"
    args = ["-c", "sleep ${opt.delay}; echo 2"]
  }
}

job "test" {
  option "delayone" {
    type = number
  }

  option "delaytwo" {
    type = number
  }

  option "concurrency" {
    type = number
  }

  concurrency = opt.concurrency

  step "one" {
    run "one" {
      delay = opt.delayone
    }
  }

  step "two" {
    run "two" {
      delay = opt.delaytwo
    }
  }
}
