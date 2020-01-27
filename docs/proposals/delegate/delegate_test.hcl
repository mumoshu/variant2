// test for `job "test"`
test "test" {
  case "case1" {
    exitstatus = 0
    err = ""
    out = "message1"
  }

  run "test" {
  }

  assert "error" {
    condition = run.err == case.err
  }

  assert "out" {
    condition = (run.res.set && run.res.stdout == case.out) || !run.res.set
  }

  assert "exitstatus" {
    condition = (run.res.set && run.res.exitstatus == case.exitstatus) || !run.res.set
  }
}
