job "bar baz" {
  description = "desc for bar/baz"

  parameter param2 {
    type = string
    description = "param2"
  }

  run {

    phase {
      name = "phase1"

      run {
        concurrency = 1

        step {
          name = "foo"
          script = "bar/baz/foo param1=${param.param1},param2=${param.param2}"
        }

        step {
          name = "bar"
          script = "bar/baz/bar: param1=${param.param1},param2=${param.param2}"
        }

        step {
          name = "baz"
          job "cmd1" {
            param2 = "cmd1param2"
          }
          script = "bar/baz/baz"
        }

        step {
          name = "aggregate"
          need = [
            "foo",
            "bar",
            "baz",
          ]

          runner {}
          //
          //          dynamic "step" {
          //            for_each = [step.baz.stdout]
          //            iterator = nested
          //            content {
          //              script = "bar/baz/baz foo=${step.foo.stdout},bar=${step.bar.stdout},baz=${nested.value}"
          //            }
          //          }

          script = "bar/baz/baz foo=${step.foo.stdout},bar=${step.bar.stdout},baz=${step.baz.stdout}"
        }
      }
    }

    phase {
      needs = [
        "phase1",
      ]

      run {
        step {
          script = "phase1.foo=${phase.phase1.step.foo.stdout}"
        }
      }
    }
  }
}
