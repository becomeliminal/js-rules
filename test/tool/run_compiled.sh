#!/bin/sh
set -eu
cd "$(dirname "$0")"
got=$(third_party/node/node/bin/node test/tool/hello.js)
want="hello please, hello please"
[ "$got" = "$want" ] || { echo "expected '$want', got '$got'" >&2; exit 1; }
echo "ok  compiled output runs: $got"
