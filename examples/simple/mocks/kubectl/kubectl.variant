option namespace {
  type = string
  short = "n"
}

option file {
  type = string
  short = "f"
}

job "apply" {
  variable d {
    type = string
    value = join("", list(dirname(dirname(context.sourcedir)), "/manifests/"))
  }

  assert "args" {
    condition = (abspath(opt.file) == abspath(var.d)) && (opt.namespace == "default")
  }
}
