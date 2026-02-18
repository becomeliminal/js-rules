const { strict: assert } = require("node:assert");

// PLZ_API_URL from .env.production overrides .env (bundle mode = production)
assert.equal(import.meta.env.PLZ_API_URL, "https://prod.example.com");

// PLZ_APP_NAME from .env (no production override)
assert.equal(import.meta.env.PLZ_APP_NAME, "TestApp");

// DB_PASSWORD should NOT be exposed (no PLZ_ prefix)
assert.equal(typeof import.meta.env?.DB_PASSWORD, "undefined");

// Auto-injected defaults from Feature 01 still work
assert.equal(import.meta.env.MODE, "production");
assert.equal(import.meta.env.PROD, true);

console.log("env file test passed");
