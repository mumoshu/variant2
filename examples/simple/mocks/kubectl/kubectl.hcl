option namespace {
  type = string
  short = "n"
}

option file {
  type = string
  short = "f"
}

job "apply" {
  assert = (param.f == basename(context.sourcedir) + "/manifests/") && (param.namespace == "default")
}
