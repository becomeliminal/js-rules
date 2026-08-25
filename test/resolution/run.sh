#!/bin/sh
set -eu
cd "$(dirname "$0")"
mkdir -p _run
cp -R third_party/js/node_modules _run/node_modules
cp test/resolution/resolve_test.js _run/
cd _run && exec ../third_party/node/node/bin/node resolve_test.js
