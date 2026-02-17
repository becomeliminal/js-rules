const { strict: assert } = require("node:assert");
const { readFileSync } = require("fs");

assert.equal(typeof readFileSync, "function");
console.log("external test passed");
