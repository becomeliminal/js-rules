const { test } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");

const manifest = (name) => JSON.parse(fs.readFileSync(`test/sideeffects/${name}/pkg/package.json`, "utf8"));

test("globs are written as a list, in the package and in its declarations twin", () => {
  assert.deepStrictEqual(manifest("styled").sideEffects, ["**/*.css"]);
  assert.deepStrictEqual(manifest("styled_types").sideEffects, ["**/*.css"]);
});

test("False is written as false, not as a string", () => {
  assert.strictEqual(manifest("pure").sideEffects, false);
});

test("unset writes no field", () => {
  assert.ok(!("sideEffects" in manifest("unstated")));
});

test("the rest of the manifest is as it was", () => {
  assert.strictEqual(manifest("styled").name, "@test/styled");
  assert.ok(manifest("styled").exports["."]);
});
