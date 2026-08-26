const { test } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");

test("a stamped version names the commit it was built from", () => {
  const m = JSON.parse(fs.readFileSync("test/publish/stamped/package.json", "utf8"));
  // git describe with no tags is the abbreviated sha; with tags, tag-n-gsha.
  // Either way it is not the literal placeholder and not empty.
  assert.match(m.version, /^0\.0\.0-[0-9a-zA-Z._-]+$/, m.version);
  assert.ok(!m.version.includes("{describe}"), "the placeholder never expanded");
});
