job "foo" {
  import = "${context.sourcedir}/foo"
}

job "test" {
  run "foo bar" {
    message = "message1"
  }
}
