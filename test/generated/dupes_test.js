// A generated tree, resolving. lib-dupes binds the same package under two
// names at two versions: '@aspect-test/c' at 2.0.2 and '@aspect-test/c1'
// aliased to 1.0.0. Both must resolve to their own version.
const assert = require("node:assert/strict");

const two = require("@aspect-test/c/package.json");
const one = require("@aspect-test/c1/package.json");

assert.equal(two.version, "2.0.2", `@aspect-test/c resolved to ${two.version}`);
assert.equal(one.version, "1.0.0", `@aspect-test/c1 resolved to ${one.version}`);
assert.equal(one.name, two.name, "both aliases are the same package");

console.log(`ok  ${two.name} at ${two.version} and ${one.version} under two names`);
