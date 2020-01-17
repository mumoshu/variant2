// test for `job "app deploy"`
test "test" {
  case "ok" {
    out = trimspace(<<EOS
version.BuildInfo{Version:"v3.0.0", GitCommit:"e29ce2a54e96cd02ccfce88bee4f58bb6e2a28b6", GitTreeState:"clean", GoVersion:"go1.13.4"}
EOS
    )
    exitstatus = 0
  }

  run "test" {
  }

  assert "exitstatus" {
    condition = run.res.exitstatus == case.exitstatus
  }

  assert "out" {
    condition = run.res.stdout == case.out
  }
}
