#!/bin/sh
set -eu
BIN=$(find . -name "please_js" -type f | head -1)
[ -n "$BIN" ] || { echo "no released binary staged" >&2; exit 1; }
"$BIN" --help >/dev/null 2>&1 || [ $? = 1 ] || { echo "the released binary does not run" >&2; exit 1; }
"$BIN" packages --dir . >/dev/null || { echo "a real subcommand failed" >&2; exit 1; }
echo ok
