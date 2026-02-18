const { strict: assert } = require("node:assert");

// These should be auto-injected by please_js without explicit defines.
// js_test bundles via the bundle command, which uses mode="production".
assert.equal(import.meta.env.MODE, "production");
assert.equal(import.meta.env.PROD, true);
assert.equal(import.meta.env.DEV, false);
assert.equal(import.meta.env.BASE_URL, "/");
assert.equal(import.meta.env.SSR, false);
assert.equal(process.env.NODE_ENV, "production");

console.log("env auto-injection test passed");
