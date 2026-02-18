import logo from "@/assets/test.png";

const { strict: assert } = require("node:assert");
assert.equal(typeof logo, "string", "asset import should be a string");
assert.ok(logo.endsWith(".png"), `expected path ending in .png, got: ${logo}`);
console.log("asset_dir test passed");
