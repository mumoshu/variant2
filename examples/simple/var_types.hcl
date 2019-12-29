variable "foo1" {
  type = tuple([string])
  value = yamldecode("foo: {\"bar\":[\"BAR\"]}").foo.bar
}

variable "foo2" {
  type = list(string)
  value = yamldecode("foo: {\"bar\":[\"BAR\"]}").foo.bar
}
