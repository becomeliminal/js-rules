const { test } = require("node:test");
const assert = require("node:assert");
const { execFileSync } = require("node:child_process");
const fs = require("node:fs");

// The keystone, checked by running what tsc produced. A file that exists and
// does not run is not a compiled program.
test("tsc's output is JavaScript that runs", () => {
  const node = fs.globSync("**/node/bin/node")[0];
  assert.ok(node, "no node toolchain staged");
  const got = execFileSync(node, ["test/tool/hello.js"], { encoding: "utf8" }).trim();
  assert.equal(got, "hello please, hello please");
});
