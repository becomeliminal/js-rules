#!/bin/bash
set -euo pipefail

# Tests run in a sandbox under plz-out/tmp — the dev server needs the repo root.
REPO_ROOT="${PWD%%/plz-out/*}"
cd "$REPO_ROOT"

PORT=13200
BASE="http://localhost:$PORT"

# Start the dev server in the background
plz-out/bin/test/esm_dev_regression/dev.sh &
SERVER_PID=$!
trap "kill $SERVER_PID 2>/dev/null; wait $SERVER_PID 2>/dev/null" EXIT

# Wait for server to be ready (up to 30s)
echo "Waiting for dev server on port $PORT..."
for i in $(seq 1 60); do
    if curl -sf "$BASE/" > /dev/null 2>&1; then
        echo "Server ready after ~$((i / 2))s"
        break
    fi
    if ! kill -0 $SERVER_PID 2>/dev/null; then
        echo "FAIL: dev server exited prematurely"
        exit 1
    fi
    sleep 0.5
done

# Verify we actually connected
if ! curl -sf "$BASE/" > /dev/null 2>&1; then
    echo "FAIL: server never became ready"
    exit 1
fi

# A: Fetch HTML and verify import map is injected
HTML=$(curl -sf "$BASE/")
if ! echo "$HTML" | grep -q '<script type="importmap">'; then
    echo "FAIL: no import map injected in HTML"
    exit 1
fi
echo "PASS: import map present in HTML"

# B: Fetch the transformed main.tsx and verify it has imports
MAIN_JS=$(curl -sf "$BASE/main.tsx")
if ! echo "$MAIN_JS" | grep -q 'from "react"'; then
    echo "FAIL: transformed main.tsx missing react import"
    echo "--- first 20 lines ---"
    echo "$MAIN_JS" | head -20
    exit 1
fi
echo "PASS: main.tsx transforms correctly with imports preserved"

# C: Extract all non-prefix dep URLs from the import map and fetch each one
DEP_URLS=$(echo "$HTML" | python3 -c "
import json, sys, re
html = sys.stdin.read()
match = re.search(r'<script type=\"importmap\">(.*?)</script>', html, re.DOTALL)
if not match:
    print('ERROR: could not find importmap in HTML', file=sys.stderr)
    sys.exit(1)
m = json.loads(match.group(1))
for v in sorted(set(m['imports'].values())):
    if not v.endswith('/'):
        print(v)
")
TOTAL=$(echo "$DEP_URLS" | wc -l)
echo "Fetching $TOTAL dep URLs from import map..."

FAILURES=0
FAIL_LOG=$(mktemp)
echo "$DEP_URLS" | xargs -P 20 -I{} bash -c '
    url="$1"
    status=$(curl -s -o /dev/null -w "%{http_code}" "'"$BASE"'${url}")
    if [ "$status" != "200" ]; then
        echo "FAIL: ${url} → HTTP $status" >> '"$FAIL_LOG"'
    fi
' _ {}

if [ -s "$FAIL_LOG" ]; then
    FAILURES=$(wc -l < "$FAIL_LOG")
    cat "$FAIL_LOG"
fi
rm -f "$FAIL_LOG"

PASSED=$((TOTAL - FAILURES))
echo "$PASSED/$TOTAL dep URLs returned HTTP 200"

if [ "$FAILURES" -gt 0 ]; then
    echo "$FAILURES dep(s) FAILED"
    exit 1
fi

# D: Verify bare imports in bundled dep files actually resolve via the browser
# import map. For each bare import, resolve it (exact match or prefix) against
# the LIVE import map from the HTML, then fetch the resolved URL. This catches:
# - Missing transitive deps (no import map entry at all)
# - Broken moduleconfig paths (entry exists but resolves to /@lib/ → 404)
# Skips packages not installed in node_modules (phantom externals).
echo "Checking bare imports in dep files resolve correctly..."

PREBUNDLE_DIR="plz-out/gen/test/esm_dev_regression/_dev_prebundle/deps"
NODE_MODULES="plz-out/gen/test/esm_dev_regression/npm_deps"

# Write HTML to temp file to avoid shell quoting issues with large strings.
HTML_FILE=$(mktemp)
echo "$HTML" > "$HTML_FILE"

# Use Python: parse HTML import map, scan dep files, resolve each bare import
# via the import map, collect unique resolved URLs to fetch.
URL_FILE=$(mktemp)
python3 << PYEOF
import json, sys, re, subprocess, os

with open("$HTML_FILE") as f:
    html = f.read()
match = re.search(r'<script type="importmap">(.*?)</script>', html, re.DOTALL)
if not match:
    print("ERROR: no importmap in HTML", file=sys.stderr)
    sys.exit(1)
im = json.loads(match.group(1))["imports"]

# Build exact and prefix lookup
exact = {}
prefixes = {}
for k, v in im.items():
    if k.endswith("/"):
        prefixes[k] = v
    else:
        exact[k] = v

def resolve_spec(spec):
    if spec in exact:
        return exact[spec]
    best_k, best_v = "", ""
    for k, v in prefixes.items():
        if spec.startswith(k) and len(k) > len(best_k):
            best_k, best_v = k, v
    if best_k:
        return best_v + spec[len(best_k):]
    return None

# Extract bare imports from dep files
# Static imports: from "pkg"
r1 = subprocess.run(
    ["grep", "-roh", 'from "[a-zA-Z@][^"]*"',
     "$PREBUNDLE_DIR", "--include=*.js"],
    capture_output=True, text=True
)
# Dynamic imports: import("pkg")
r2 = subprocess.run(
    ["grep", "-roh", 'import("[a-zA-Z@][^"]*")',
     "$PREBUNDLE_DIR", "--include=*.js"],
    capture_output=True, text=True
)
specs = set()
for line in r1.stdout.strip().split("\n"):
    if line:
        specs.add(line.replace('from "', "").rstrip('"'))
for line in r2.stdout.strip().split("\n"):
    if line:
        specs.add(line.replace('import("', "").rstrip('")'))

nm = "$NODE_MODULES"
urls = set()
unresolved = []
for spec in sorted(specs):
    if spec.startswith("@"):
        parts = spec.split("/", 2)
        pkg = parts[0] + "/" + parts[1] if len(parts) >= 2 else spec
    else:
        pkg = spec.split("/")[0]
    if not os.path.isdir(os.path.join(nm, pkg)):
        continue
    url = resolve_spec(spec)
    if url is None:
        unresolved.append(spec)
    elif not url.endswith("/"):
        urls.add(url)

with open("$URL_FILE", "w") as f:
    if unresolved:
        f.write("UNRESOLVED:" + ",".join(unresolved) + "\n")
    for u in sorted(urls):
        f.write(u + "\n")

print(f"Resolved {len(urls)} unique dep URLs from {len(specs)} bare imports ({len(specs) - len(urls) - len(unresolved)} skipped as uninstalled)")
if unresolved:
    print(f"WARNING: {len(unresolved)} unresolved: {', '.join(unresolved[:5])}...")
PYEOF
rm -f "$HTML_FILE"

# Check for unresolved bare imports
if grep -q '^UNRESOLVED:' "$URL_FILE"; then
    UNRESOLVED=$(grep '^UNRESOLVED:' "$URL_FILE" | sed 's/^UNRESOLVED://')
    echo "FAIL: bare imports with no import map entry: $UNRESOLVED"
    rm -f "$URL_FILE"
    exit 1
fi

URL_COUNT=$(grep -c '^/' "$URL_FILE" || true)
echo "Fetching $URL_COUNT resolved dep URLs..."

FAIL_LOG=$(mktemp)
{ grep '^/' "$URL_FILE" || true; } | xargs -P 20 -I{} bash -c '
    url="$1"
    status=$(curl -s -o /dev/null -w "%{http_code}" "'"$BASE"'${url}")
    if [ "$status" != "200" ]; then
        echo "FAIL: ${url} → HTTP $status" >> '"$FAIL_LOG"'
    fi
' _ {}
rm -f "$URL_FILE"

if [ -s "$FAIL_LOG" ]; then
    FAILURES=$(wc -l < "$FAIL_LOG")
    echo "--- $FAILURES resolved dep URLs failed ---"
    cat "$FAIL_LOG"
    rm -f "$FAIL_LOG"
    exit 1
fi
rm -f "$FAIL_LOG"
echo "PASS: all bare imports in dep files resolve and return HTTP 200"

echo "ALL CHECKS PASSED"
