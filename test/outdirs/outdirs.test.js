const { test } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");

const pkg = "test/outdirs";

test("an out_dirs directory's contents are the outputs, at the package root", () => {
  assert.match(fs.readFileSync(`${pkg}/a.js`, "utf8"), /"a"/);
  assert.match(fs.readFileSync(`${pkg}/sub/b.js`, "utf8"), /"b"/);
});

test("the out_dirs directory itself is not a second copy of them", () => {
  assert.strictEqual(fs.existsSync(`${pkg}/out`), false);
});

test("a directory source reached the tool with its contents", () => {
  assert.match(fs.readFileSync(`${pkg}/c.js`, "utf8"), /"c"/);
});
