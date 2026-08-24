#!/bin/sh
set -eu
cd "$(dirname "$0")"
mkdir -p _run
cp -R test/generated/node_modules_lib-dupes _run/node_modules
cp test/generated/dupes_test.js _run/
cd _run && exec ../third_party/node/node/bin/node dupes_test.js
