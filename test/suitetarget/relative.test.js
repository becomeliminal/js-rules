const { test } = require("node:test");
const assert = require("node:assert");
const { greet } = require("./helper.js");

test("a built test imports its neighbour by relative path", () => {
  assert.strictEqual(greet("ada"), "Hello, ada!");
});
