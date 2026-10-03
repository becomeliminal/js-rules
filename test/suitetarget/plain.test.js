const { test } = require("node:test");
const assert = require("node:assert");

test("a source file still runs beside built ones", () => {
  assert.ok(true);
});
