FROM alpine:3.10

ARG HELM_VERSION=3.0.0
ARG HELM_FILENAME="helm-${HELM_VERSION}-linux-amd64.tar.gz"

ADD http://storage.googleapis.com/kubernetes-helm/${HELM_FILE_NAME} /tmp
RUN tar -zxvf /tmp/${HELM_FILE_NAME} -C /tmp \
  && mv /tmp/linux-amd64/helm /bin/helm \
  && rm -rf /tmp/* \
  && /bin/helm init --client-only
