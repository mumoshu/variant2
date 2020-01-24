job "foo bar" {
  parameter "message" {
    type = string
  }

  exec {
    command = "echo"
    args= [param.message]
  }
}
