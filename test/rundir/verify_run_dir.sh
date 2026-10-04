#!/bin/sh
# js_binary with an entry among linked sources, and link_dirs, scripted. A run
# target writing into the repository, so not a plz test. From the repo root:
#
#   ./test/rundir/verify_run_dir.sh
set -u
DIR=test/rundir
OUT=$DIR/out/rendered.txt
fail=0
check() { if [ "$2" = "$3" ]; then printf "  %-56s ok\n" "$1"; else printf "  %-56s FAIL (wanted %s, got %s)\n" "$1" "$2" "$3"; fail=1; fi; }
rm -rf "$DIR/out"
trap 'rm -rf "$DIR/out"; cp "$DIR/src/input.txt.bak" "$DIR/src/input.txt" 2>/dev/null; rm -f "$DIR/src/input.txt.bak"' EXIT
cp "$DIR/src/input.txt" "$DIR/src/input.txt.bak"

plz run //$DIR:render >/dev/null 2>&1
check "the program ran and wrote into the repository's out/" 1 "$(grep -c RUNDIR_INPUT_MARKER "$OUT" 2>/dev/null || echo 0)"
check "  ... importing a package from the tree beside it" 1 "$(grep -c '1m' "$OUT" 2>/dev/null || echo 0)"

# A linked source: an edit is read by the next run without a rebuild.
printf 'EDITED_MARKER\n' > "$DIR/src/input.txt"
plz run //$DIR:render >/dev/null 2>&1
check "an edited source is read without a rebuild" 1 "$(grep -c EDITED_MARKER "$OUT" 2>/dev/null || echo 0)"

[ "$fail" = 0 ] && echo "PASS" || echo "FAIL"
exit "$fail"
