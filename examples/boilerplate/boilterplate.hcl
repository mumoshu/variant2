boilerplate "commons" {
  parameter "param1" {
    type = string
  }

  option "opt1" {
    type = string
  }

  variable "var1" {
    value = "${param.param1} + ${opt.opt1}"
  }
}

job "test" {
  // This is replaced to parameters, options and variables defined in the named boilerplate
  boilerplate = "commons"

  exec {
    command = "echo"
    // So that we can refer to var1 defined in the boilerplate here
    args = [var.var1]
  }
}
