job "test" {
  module = "default"

  exec {
    command = "sh"
    args = ["-c", "helm3 version -c"]
    env = {
      PATH = "${mod.pathaddition}:/bin:/usr/bin:/usr/local/bin"
    }
  }
}
