#!/usr/bin/env bash

PROJECT_ROOT=../../..
VARIANT=${PROJECT_ROOT}/variant

export VARIANT_BUILD_VER=v0.33.3
export VARIANT_BUILD_REPLACE=$(pwd)/${PROJECT_ROOT}

rm -rf ../exported
rm -rf ../compiled

(cd ${PROJECT_ROOT}; make build)
${VARIANT} export go ../import-multi ../exported
${VARIANT} export binary ../import-multi ../compiled

${VARIANT} run foo baz HELLO1

(cd ..; ./compiled foo baz HELLO2)
