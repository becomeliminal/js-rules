#!/bin/sh
set -eu
D=test/capture

for f in out.txt err.txt status; do
  [ -f "$D/$f" ] || { echo "$f was not captured" >&2; exit 1; }
done

# The build succeeded even though the tool did not: that is what exit_code_out
# buys, and without it this target would fail rather than produce anything.
[ "$(cat "$D/status")" != "0" ] \
  || { echo "expected a non-zero status, got $(cat "$D/status")" >&2; exit 1; }

# tsc reports an unknown flag on stdout; the point is that whichever stream it
# chose, it was captured rather than lost.
grep -qi "nonsense" "$D/out.txt" "$D/err.txt" \
  || { echo "the tool's diagnostic was not captured on either stream" >&2; exit 1; }

echo "ok"
