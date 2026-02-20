package esmdev

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

// nodeDetectScript is the Node.js script that requires each entry point
// and returns its export names via Object.keys(). It stubs browser globals
// to prevent side-effect crashes in packages that access window/document/navigator
// at require time.
const nodeDetectScript = `
var e = JSON.parse(process.argv[1]);
var r = {};
// Stub browser globals to prevent crashes in packages with side effects
if (typeof globalThis.window === 'undefined') globalThis.window = {};
if (typeof globalThis.document === 'undefined') globalThis.document = { createElement: function() { return {}; }, addEventListener: function() {} };
if (typeof globalThis.navigator === 'undefined') globalThis.navigator = { userAgent: '' };
if (typeof globalThis.self === 'undefined') globalThis.self = globalThis;
for (var k in e) {
  try {
    var m = require(e[k]);
    r[k] = Object.keys(m).filter(function(n) { return n !== '__esModule' && n !== 'default'; });
  } catch(ex) { r[k] = null; }
}
process.stdout.write(JSON.stringify(r));
`

// detectCJSExports runs Node.js to require() each entry point and enumerate
// its exports via Object.keys(). Returns a map of specifier → export names.
// Entries that fail to require (ESM-only packages, missing deps) return nil
// and the caller falls back to regex detection for those.
//
// If Node is unavailable or the script fails entirely, returns nil, nil —
// the caller falls back to regex for all entries.
func detectCJSExports(nodePath string, entryPoints map[string]string) (map[string][]string, error) {
	if len(entryPoints) == 0 {
		return nil, nil
	}

	entriesJSON, err := json.Marshal(entryPoints)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, nodePath, "-e", nodeDetectScript, string(entriesJSON))
	out, err := cmd.Output()
	if err != nil {
		return nil, nil // graceful fallback
	}

	var result map[string][]string
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, nil // graceful fallback
	}

	return result, nil
}
