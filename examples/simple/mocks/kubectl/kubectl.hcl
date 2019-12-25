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
  step {
    assert = opt.file == var.d
  }
  step {
    assert = (opt.namespace == "default")
  }
}
