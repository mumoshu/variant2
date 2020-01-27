// test for `job "app deploy"`
test "options" {
  case "ok1" {
    exitstatus = 0
    err = ""
    out = trimspace(<<EOS
1 2 3 a b|c
EOS
    )
  }

  run "test" {
    int1 = 1
    ints1 = list(2,3)
    str1 = "a"
    strs1 = list("b","c")
  }

  assert "error" {
    condition = run.err == case.err
  }

  assert "out" {
    condition = (run.res.set && run.res.stdout == case.out) || !run.res.set
  }

  assert "exitstatus" {
    condition = run.res.exitstatus == case.exitstatus
  }
}
