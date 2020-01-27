option "int1" {
  type = number
}

option "ints1" {
  type = list(number)
}

option "str1" {
  type = string
}

option "strs1" {
  type = list(string)
}

job "test" {
  exec {
    command = "echo"
    args = [
      tostring(opt.int1),
      tostring(opt.ints1[0]),
      tostring(opt.ints1[1]),
      opt.str1,
      join("|", opt.strs1)
    ]
  }
}
