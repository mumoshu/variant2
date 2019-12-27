job "ruby" {
  option "script" {
    type = string
  }

  exec {
    command = "ruby"

    args = [
      "-e",
      opt.script
    ]

    env = {
      SCRIPT = opt.script
    }
  }
}

job "test1" {
  run "ruby" {
    script = "puts \"TEST\""
  }
}

job "test2" {
  step "foo" {
    run "ruby" {
      script = "puts \"TEST\""
    }
  }
}

job "test3" {
  run "test1" {

  }
}
