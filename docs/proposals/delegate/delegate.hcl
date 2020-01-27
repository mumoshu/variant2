job "foo" {
  delegate = "${context.sourcedir}/foo"
}

job "test" {
  run "foo bar" {
    message = "message1"
  }
}
