const { test } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");

const dir = "test/publish/typed/package";

test("a library's declarations ship beside its sources", () => {
  // GIVEN a library with index.js and the declaration index.d.ts
  // WHEN it is packaged for a registry
  const files = fs.readdirSync(dir).sort();

  // THEN the package holds both, and the manifest a registry reads
  assert.deepEqual(files, ["index.d.ts", "index.js", "package.json"]);
});

test("the manifest's types entry names a file the package ships", () => {
  // GIVEN the packaged library
  // WHEN a compiler resolves the package by its manifest
  const manifest = JSON.parse(fs.readFileSync(`${dir}/package.json`, "utf8"));

  // THEN its types entry, in both places a compiler looks, is that declaration
  assert.equal(manifest.types, "index.d.ts");
  assert.equal(manifest.exports["."].types, "./index.d.ts");
  assert.ok(fs.existsSync(`${dir}/${manifest.types}`), "types entry names a missing file");
});
