#!/usr/bin/env bash
set -e
cd $(dirname "$0")

if [ "$(uname)" == "Darwin" ]; then
	export GOOS="${GOOS:-linux}"
fi

ORG_PATH="github.com/Mellanox"
export REPO_PATH="${ORG_PATH}/infiniband-cni"

if [ ! -h gopath/src/${REPO_PATH} ]; then
	mkdir -p gopath/src/${ORG_PATH}
	ln -s ../../../.. gopath/src/${REPO_PATH} || exit 255
fi

export GOPATH=${PWD}/gopath
export GO="${GO:-go}"
export GOFLAGS="${GOFLAGS} -mod=vendor"

mkdir -p "${PWD}/bin"

echo "Building Infiniband IPoIB Plugin ${GOOS}"
$GO build -o "${PWD}/bin/infiniband" "$@" "$REPO_PATH"
echo "Finished Successfuly"