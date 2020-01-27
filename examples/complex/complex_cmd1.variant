job "cmd1" {
  description = "desc for bar/baz"

  parameter param2 {
    type = string
    description = "param2"
  }

  step "one" {
    run "echo" {
      script = "cmd1 param1=${param.param1},param2=${param.param2}"
    }
  }
}
