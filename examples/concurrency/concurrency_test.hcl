test "test" {
  case "ok1" {
    err = ""
    stdout = "2"
    delayone = 1
    delaytwo = 0
  }

  case "ok2" {
    err = ""
    stdout = "2"
    delayone = 0
    delaytwo = 1
  }

  run "test" {
    delayone = case.delayone
    delaytwo = case.delaytwo
  }

  assert "err" {
    condition = run.err == case.err
  }

  assert "stdout" {
    condition = run.res.stdout == case.stdout
  }
}
