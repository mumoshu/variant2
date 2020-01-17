// test for `job "app deploy"`
test "test" {
  case "ok" {
    out = trimspace(<<EOS
version.BuildInfo{Version:"v3.0.0", GitCommit:"e29ce2a54e96cd02ccfce88bee4f58bb6e2a28b6", GitTreeState:"clean", GoVersion:"go1.13.4"}
EOS
    )
    exitstatus = 0
  }

  run "test" {
  }

  assert "exitstatus" {
    condition = run.res.exitstatus == case.exitstatus
  }

  assert "out" {
    condition = run.res.stdout == case.out
  }
}

test "build" {
  case "ok" {
    out = trimspace(<<EOS
FROM alpine:3.10

ARG HELM_VERSION=3.0.0
ARG HELM_FILENAME="helm-$${HELM_VERSION}-linux-amd64.tar.gz"

ADD http://storage.googleapis.com/kubernetes-helm/$${HELM_FILE_NAME} /tmp
RUN tar -zxvf /tmp/$${HELM_FILE_NAME} -C /tmp \
  && mv /tmp/linux-amd64/helm /bin/helm \
  && rm -rf /tmp/* \
  && /bin/helm init --client-only
EOS
    )
    exitstatus = 0
  }

  run "build" {
  }

  assert "exitstatus" {
    condition = run.res.exitstatus == case.exitstatus
  }

  assert "out" {
    condition = run.res.stdout == case.out
  }
}
