const { test } = require("node:test");
const assert = require("node:assert");

// Fetched by its exact URL rather than registry construction -- the path every
// npm-lockfile package and every private-registry package takes.
test("a package fetched by exact URL is a package like any other", () => {
  assert.equal(require("ms")(60000), "1m");
});
