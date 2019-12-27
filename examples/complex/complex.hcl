description = "desc for default"

config {
  files = [
    "foo.yaml"]
  // contexts = ["prod", "cluster1"]
  // directories = ["config"]
}

parameter def1 {
  description = "defa1"
  type = string
  default = try(jsonpath(file("foo.json"), "foo.bar"), file("foo.txt"), "def1default")
}

parameter param1 {
  description = "param1"
  type = string
  default = "aa"
}


variable lis {
  type = list(string)
  value = [
    "a",
    "b"]
}

variable mm {
  type = map(string)
  value = {
    foo = "bar"
  }
}

job "echo" {
  parameter "script" {
    type = string
  }

  exec {
    command = "echo"
    args = [
      param.script]
  }
}

job "main" {
  option opt1 {
    type = string
    description = "opt1"
  }

  variable var1 {
    type = string
    value = "var1:param1=${param.param1},opt1=${opt.opt1},def1=${param.def1}"
  }

  run "echo" {
    script = "param1=${param.param1},var1=${var.var1},mm=${var.mm["foo"]}"
  }

}
