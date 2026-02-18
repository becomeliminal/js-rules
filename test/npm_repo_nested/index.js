// Tests nested-only package promotion and version-conflict target generation.
//
// has-flag@3.0.0 exists only nested under debug — the resolver must
// promote it to a top-level target for it to be importable.
//
// ms@2.1.2 is nested under debug (top-level is ms@2.1.3) — the resolver
// should generate a version-conflict target (ms_v2_1_2) and wire it into
// debug's npm_module via nested_deps, proving the |lib output selector
// and nested_deps copy mechanism work.
const ms = require("ms");
const debug = require("debug");
const hasFlag = require("has-flag");
const { strict: assert } = require("node:assert");

// Verify ms (top-level) works
assert.equal(ms("1h"), 3600000, "ms should convert 1h to milliseconds");

// Verify has-flag (promoted from nested-only) works
assert.equal(typeof hasFlag, "function", "has-flag should export a function");

// Verify debug works (internally imports ms via moduleconfig)
const log = debug("test");
assert.equal(typeof log, "function", "debug should export a function");

console.log("npm_repo_nested test passed");
