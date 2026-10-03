const { test } = require("node:test");

// Not a *.test file: a directory source must not contribute it.
test("a non-test file in a built directory is never run", () => {
  throw new Error("fixture.js was run as a test");
});
