#!/usr/bin/env bash
# Shell script to build the driver.
# Used both for development builds and release / cross-builds.
# It is meant to be called from the Makefile.
#
# There are a number of environment variables and command line
# options which must be set for this build script to work properly.
# The environment variables are set by the appropriate development
# shells (ie. nix/crossBuild.nix and nix/devShell.nix). The command line
# options are set when the script is invoked in the Makefile. There is
# nothing more to be done in this regard, the information is simply here
# to provide clarity.
# 
# Environment variables:
# - GOOS: target OS (eg. linux)
# - GOARCH: target (eg. amd64)
# - CC: path to the C compiler
# - STATIC_BUILD: whether to link statically or dynamically,
#                 use "1" to enable static linking
#
# Optionally, use VERBOSE=1 to make the script echo the above variables.
#
# Command line options:
# - v: driver version
# - i: path to main.go
# - o: path to output
#
# Usage: build.sh -v <version> -i <input> -o <output>

set -euo pipefail

IN=""
OUT=""
VERSION=""
BUILD_TAGS=""

while getopts "i:o:v:t:" opt; do
  case $opt in
    i) IN="$OPTARG"
       ;;
    o) OUT="$OPTARG"
       ;;
    v) VERSION="$OPTARG"
       ;;
    t) BUILD_TAGS="$OPTARG"
       ;;
    \?) echo "Invalid option: -$OPTARG" >&2; exit 1 ;;
  esac
done

all_flags_set=true
ensure_flag_set() {
  local flag_name="$1"
  local flag_value="$2"
  if [ -z "$flag_value" ]; then
    echo "$flag_name not set"
    all_flags_set=false
  fi
}

ensure_flag_set "-v" "$VERSION"
ensure_flag_set "-i" "$IN"
ensure_flag_set "-o" "$OUT"

if [ "$all_flags_set" = false ]; then
  echo "Usage: build-driver -v <version> -i <input> -o <output> [-t <tags>]"
  exit 1
fi

LD_VERSION="-X github.com/dividat/driver/src/dividat-driver/server.version=$VERSION"
STATIC_LINKING_LDFLAGS=""

if [ "${STATIC_BUILD:-0}" = "1" ]; then
  STATIC_LINKING_LDFLAGS="-linkmode external -extldflags \"-static\""
fi

LD_FLAGS="$LD_VERSION $STATIC_LINKING_LDFLAGS"

VERBOSE=${VERBOSE:-"0"}

if [ $VERBOSE = "1" ]; then
  echo "GOOS=${GOOS:-}"
  echo "GOARCH=${GOARCH:=}"
  echo "GCO_ENABLED=${CGO_ENABLED:=}"
  echo "CC=${CC:=}"
  echo "LD_FLAGS=$LD_FLAGS"
  echo "BUILD_TAGS=$BUILD_TAGS"
fi

go build -tags "$BUILD_TAGS" -ldflags "$LD_FLAGS" -o "$OUT" "$IN"

echo "Built $OUT"
