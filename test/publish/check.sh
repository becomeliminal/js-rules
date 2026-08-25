#!/bin/sh
set -eu
P=test/publish/package/package.json

grep -q '"version": "1.4.2"' "$P" || { echo "version not published: $(cat "$P")" >&2; exit 1; }
grep -q '"license": "Apache-2.0"' "$P" || { echo "license missing" >&2; exit 1; }
# A value that parses as JSON stays JSON, rather than becoming a string that
# looks like one -- npm reads repository as an object.
grep -q '"type": "git"' "$P" || { echo "repository was flattened to a string: $(cat "$P")" >&2; exit 1; }

# And what the library produced survives untouched. The exports map is the one
# that matters: without it a package is unimportable under ESM, and neither
# failure shows up until someone installs it.
grep -q '"name": "@test/publishable"' "$P" || { echo "name was rewritten" >&2; exit 1; }
grep -q '"exports"' "$P" || { echo "the exports map was lost" >&2; exit 1; }
[ -f test/publish/package/README.md ] || { echo "README not included" >&2; exit 1; }
[ -f test/publish/package/index.js ] || { echo "sources missing" >&2; exit 1; }

# The test that matters, and the one that caught the missing exports map when
# this layer was built: stage it as a dependency and import it by name. A
# package.json that looks right and cannot be imported passes every check above.
NODE=$(find . -path "*/node/bin/node" -type f | head -1)
[ -n "$NODE" ] || { echo "no node toolchain staged" >&2; exit 1; }
mkdir -p sandbox/node_modules/@test
cp -R test/publish/package sandbox/node_modules/@test/publishable
cat > sandbox/main.js <<'EOF'
const { hello } = require("@test/publishable");
if (hello() !== "published") throw new Error("wrong value: " + hello());
EOF
(cd sandbox && "$OLDPWD/$NODE" main.js) \
  || { echo "the published package cannot be imported by name" >&2; exit 1; }

echo "ok"
