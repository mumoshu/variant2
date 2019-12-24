parameter namespace {
  type = string
  short = "n"
}

parameter file {
  type = string
  short = "f"
}

job "apply" {
  assert = (param.f == basename(context.sourcedir) + "/manifests/") && (param.namespace == "default")
}
