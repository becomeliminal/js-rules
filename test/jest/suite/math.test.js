const { add } = require("./math");

describe("a suite already written in jest", () => {
  test("adds", () => {
    expect(add(2, 2)).toBe(4);
  });

  test("matches its committed snapshot", () => {
    // The snapshot is committed beside the test; under --ci a mismatch fails
    // rather than rewriting it, which is the property jest_test exists to pin.
    expect({ shape: add(1, 1) }).toMatchSnapshot();
  });
});
