namespace "foo" {
  # `variant run foo` shows this description
  description = "collection of jobs related to foo"

  # `variant run foo -h` shows opt1 as a global flag
  option "opt1" {
    type = string
  }

  # mostly equivalent to job "foo test", but with opt1 as the persistent flag
  # i.e. `variant run foo bar -h` shows opt1 as a global flag
  job "test" {
    exec {
      command = "echo"
      args = "${opt.opt1}"
    }
  }
}
