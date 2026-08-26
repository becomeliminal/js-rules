const { test, describe } = require("node:test");
const assert = require("node:assert");

// A first-party library, imported by package name through the tree Please
// staged. It resolves react from the store in turn, so this one import
// exercises both halves of the graph.
const { greet } = require("@test/greeter");

describe("greet", () => {
  test("greets by name", () => {
    assert.match(greet("Please"), /^hello Please /);
  });

  test("reaches its own third-party dependency", () => {
    assert.match(greet("x"), /react 18\.3\.1/);
  });
});

test("node's assertions are there without installing anything", () => {
  assert.deepEqual({ a: [1, 2] }, { a: [1, 2] });
});
