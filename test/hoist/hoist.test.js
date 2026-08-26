const { test } = require("node:test");
const assert = require("node:assert");

// @test/broken requires ms, which it never declared. ms is transitive here --
// only debug depends on it -- so the strict store correctly keeps it off the
// top level, and only the hoist makes this import resolve.
const { oneMinute } = require("@test/broken");

test("an undeclared import resolves through the hoisted package", () => {
  assert.equal(oneMinute(), "1m");
});
