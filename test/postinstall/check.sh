#!/bin/sh
set -eu

M=test/postinstall/ms/pkg/marker.json

[ -f "$M" ] || { echo "postinstall did not run: no marker in the package" >&2; exit 1; }

# node was on PATH: the command is a node one-liner, so a marker at all proves it.
# The value proves the declared environment reached it, spaces intact.
grep -q '"from":"a value with spaces"' "$M" \
  || { echo "declared environment did not reach the command: $(cat "$M")" >&2; exit 1; }

# And nothing else did. SHOULD_NOT_LEAK is exported by the test around this
# build, so a null here is the whole claim: the command sees what was named and
# not the machine it ran on.
grep -q '"leaked":null' "$M" \
  || { echo "the build's own environment leaked in: $(cat "$M")" >&2; exit 1; }

echo "ok"
