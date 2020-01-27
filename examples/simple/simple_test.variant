// test for `job "app deploy"`
test "app deploy" {
  variable "path" {
    type = string
    value = ".:/bin:/usr/bin:${abspath("${context.sourcedir}/mocks/kubectl")}"
  }

  case "ng1" {
    namespace = "foo"
    exitstatus = 1
    err = trimspace(<<EOS
command "bash -c     kubectl -n foo apply -f examples/simple/manifests/
": exit status 1
EOS
    )
  }

  case "ok1" {
    namespace = "default"
    exitstatus = 0
    err = ""
  }

  run "app deploy" {
    namespace = case.namespace
    path = var.path
  }

  assert "error" {
    condition = run.err == case.err
  }

  assert "exitstatus" {
    condition = run.res.exitstatus == case.exitstatus
  }
}
