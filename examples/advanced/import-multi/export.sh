#!/usr/bin/env bash

PROJECT_ROOT=../../..
VARIANT=${PROJECT_ROOT}/variant

export VARIANT_BUILD_VER=v0.36.0
export VARIANT_BUILD_VARIANT_REPLACE=$(pwd)/${PROJECT_ROOT}

rm -rf ../exported
rm -rf ../compiled

export GITHUB_REF=refs/heads/${VARIANT_BUILD_VER}

(cd ${PROJECT_ROOT}; make build)
${VARIANT} export go ../import-multi ../exported
${VARIANT} export binary ../import-multi ../compiled

${VARIANT} run foo baz HELLO1

(cd ..; ./compiled foo baz HELLO2)
