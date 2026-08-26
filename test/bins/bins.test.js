const { test } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");

// The build getting this far is most of the assertion: resolve-bin refuses a
// package that publishes nothing, so a status file at all means the declared
// executable was found and run.
test("a declared executable runs", () => {
  assert.equal(fs.readFileSync("test/bins/status", "utf8").trim(), "0");
});
