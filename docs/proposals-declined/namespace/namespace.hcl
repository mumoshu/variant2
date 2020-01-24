namespace "foo" {
  # `variant run foo` shows this description
  description = "collection of jobs related to foo"

  # `variant run foo -h` shows opt1 as a global flag
  option "opt1" {
    type = string
  }
}

namespace "foo bar" {
  # `variant run foo` shows this description
  description = "collection of jobs related to bar"

  # `variant run foo bar -h` shows opt2 as a global flag
  option "opt2" {
    type = string
  }
}

# opt1 and opt2 are available as persistent flags
# i.e. `variant run foo bar test -h` shows opt1 and opt2 as global flags
job "foo bar test" {
  exec {
    command = "echo"
    args = "opt1=${opt.opt1},opt2=${opt.opt2}"
  }
}
