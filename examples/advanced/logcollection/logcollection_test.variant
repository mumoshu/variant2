// test for `job "test"`
test "test" {
  case "case1" {
    exitstatus = 0
    err = ""
    out = trimspace(<<EOS
text=foo
EOS
    )
    filecontent = trimspace(<<EOS
run={"args":{"message":"foo"},"job":"echo"}
exec={"args":["text=foo"],"command":"echo"}
EOS
    )
    filepath = "${context.sourcedir}/log.txt"
  }

  run "test" {
  }

  assert "error" {
    condition = run.err == case.err
  }

  assert "out" {
    condition = (run.res.set && run.res.stdout == case.out) || !run.res.set
  }

  assert "file" {
    condition = file(case.filepath) == case.filecontent
  }

  assert "exitstatus" {
    condition = (run.res.set && run.res.exitstatus == case.exitstatus) || !run.res.set
  }
}
