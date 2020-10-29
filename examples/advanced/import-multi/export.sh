#!/usr/bin/env bash

PROJECT_ROOT=../../..
VARIANT=${PROJECT_ROOT}/variant

export VARIANT_BUILD_VER=v0.33.3
export VARIANT_BUILD_VARIANT_REPLACE=$(pwd)/${PROJECT_ROOT}
export VARIANT_BUILD_MOD_REPLACE="github.com/summerwind/whitebox-controller@v0.7.1=github.com/mumoshu/whitebox-controller@v0.5.1-0.20201028130131-ac7a0743254b"

rm -rf ../exported
rm -rf ../compiled

(cd ${PROJECT_ROOT}; make build)
${VARIANT} export go ../import-multi ../exported
${VARIANT} export binary ../import-multi ../compiled

${VARIANT} run foo baz HELLO1

(cd ..; ./compiled foo baz HELLO2)
