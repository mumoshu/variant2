job "bar baz" {
  description = "desc for bar/baz"

  parameter param2 {
    type = string
    description = "param2"
  }

  concurrency = 1

  step "foo" {
    run "echo" {
      script = "bar/baz/foo param1=${param.param1},param2=${param.param2}"
    }
  }

  step "bar" {
    run "echo" {
      script = "bar/baz/bar: param1=${param.param1},param2=${param.param2}"
    }
  }

  step "baz" {
    run "cmd1" {
      param2 = "cmd1param2"
    }
  }

  step "aggregate" {
    need = [
      "foo",
      "bar",
      "baz",
    ]

    run "echo" {
      script = "bar/baz/baz foo=${step.foo.stdout},bar=${step.bar.stdout},baz=${step.baz.stdout}"
    }
  }
}
