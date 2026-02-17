const { strict: assert } = require("node:assert");
const data = require("./data.json");

assert.equal(data.name, "js-rules");
assert.equal(data.version, "1.0.0");
console.log("json_import test passed");
