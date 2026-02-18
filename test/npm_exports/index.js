// devlop uses exports-only (no main/module fields) — tests that
// ModuleResolvePlugin correctly resolves packages via exports map.
const devlop = require("devlop");

const { strict: assert } = require("node:assert");
assert.ok(devlop, "devlop module should load");
console.log("npm_exports test passed");
