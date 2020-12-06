#!/usr/bin/env bash

set -e

tag=${GITHUB_REF##*/}

if [ -z "${tag}" ]; then
  echo GITHUB_REF must be set 1>&2
  exit 1
fi

export VERSION=${tag}
export MOD_REPLACES=$(go run hack/print-replaces.go)

if [ ! -z "${GITHUB_ENV}" ]; then
  echo "VERSION=${VERSION}" >> $GITHUB_ENV
  echo "MOD_REPLACES=${MOD_REPLACES}" >> $GITHUB_ENV
fi
