const { strict: assert } = require("node:assert");
const logo = require("./logo.svg");

assert.equal(typeof logo, "string", "asset import should be a string");
assert.ok(logo.endsWith(".svg"), `expected path ending in .svg, got: ${logo}`);
console.log("asset_import test passed");
