#!/bin/sh
# node_modules and the toolchain are staged by Please as test data at their
# package-relative paths; node runs with node_modules as a sibling of the test,
# which is the layout node's resolution algorithm expects.
set -eu
cd "$(dirname "$0")"
exec third_party/node/node/bin/node test/resolution/resolve_test.js
