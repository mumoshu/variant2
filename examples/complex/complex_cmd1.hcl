job "cmd1" {
  description = "desc for bar/baz"

  parameter param2 {
    type = string
    description = "param2"
  }

  run {
    dynamic "step" {
      for_each = ["a", "b", "c"]
      iterator = nested
      content {
        script = "dynamic block ${nested.value}"
      }
    }

    step {
      script = "cmd1 param1=${param.param1},param2=${param.param2}"
    }
  }
}
