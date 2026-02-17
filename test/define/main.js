const { strict: assert } = require("node:assert");

assert.equal(API_BASE, "https://api.example.com");
assert.equal(import.meta.env.MODE, "testing");
console.log("define test passed");
