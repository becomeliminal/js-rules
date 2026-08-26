const { test, describe } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");

// Imported by name through a tree npm_link built, which is how a consumer would
// get it -- not read out of the build directory. That is the difference between
// checking what the file says and checking that it works.
const manifest = require("@test/publishable/package.json");

describe("the manifest a registry receives", () => {
  test("carries the version it was published as", () => {
    assert.equal(manifest.version, "1.4.2");
  });

  test("carries the fields publishing needs", () => {
    assert.equal(manifest.license, "Apache-2.0");
  });

  test("keeps a JSON value as JSON rather than a string that looks like one", () => {
    // npm reads repository as an object.
    assert.equal(typeof manifest.repository, "object");
    assert.equal(manifest.repository.type, "git");
  });
});

describe("what the library produced survives", () => {
  test("the name is not rewritten", () => {
    assert.equal(manifest.name, "@test/publishable");
  });

  test("the exports map is intact", () => {
    // Without it the package is unimportable under ESM, and the failure does
    // not appear until someone installs it.
    assert.ok(manifest.exports, "the exports map was lost");
  });

  test("extra files come along", () => {
    const dir = require("node:path").dirname(require.resolve("@test/publishable/package.json"));
    assert.ok(fs.existsSync(`${dir}/README.md`), "README not included");
    assert.ok(fs.existsSync(`${dir}/index.js`), "sources missing");
  });
});

// The one that caught the missing exports map when this layer was built. A
// manifest can look entirely correct and still not resolve, and every assertion
// above passes in that case.
test("the package can be imported by name", () => {
  const { hello } = require("@test/publishable");
  assert.equal(hello(), "published");
});
