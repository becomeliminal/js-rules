import { strict as assert } from "node:assert";
import { add, multiply, factorial } from "test/js_test/lib";

// Test add
assert.equal(add(2, 3), 5);
assert.equal(add(-1, 1), 0);
assert.equal(add(0, 0), 0);

// Test multiply
assert.equal(multiply(3, 4), 12);
assert.equal(multiply(-2, 5), -10);
assert.equal(multiply(0, 100), 0);

// Test factorial
assert.equal(factorial(0), 1);
assert.equal(factorial(1), 1);
assert.equal(factorial(5), 120);
assert.equal(factorial(10), 3628800);

console.log("all math tests passed");
