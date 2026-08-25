#!/bin/sh
set -eu

# The build getting this far is most of the assertion: resolve-bin refuses a
# package that publishes nothing, so a status file at all means the declared
# executable was found and run.
[ "$(cat test/bins/status)" = "0" ] \
  || { echo "the declared executable did not run cleanly: $(cat test/bins/status)" >&2; exit 1; }

echo "ok"
