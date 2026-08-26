const { test, describe } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");

// The artifacts js_run_binary captured, read from where data stages them.
const dir = "test/capture";
const read = (f) => fs.readFileSync(`${dir}/${f}`, "utf8");

describe("capturing a tool's output", () => {
  test("writes each stream to its own declared output", () => {
    for (const f of ["out.txt", "err.txt", "status"]) {
      assert.ok(fs.existsSync(`${dir}/${f}`), `${f} was not captured`);
    }
  });

  test("records a non-zero status instead of failing the build", () => {
    // Without exit_code_out this target would not exist: the tool failed, and
    // that is the point.
    assert.notEqual(read("status").trim(), "0");
  });

  test("keeps the tool's diagnostic rather than losing it", () => {
    // tsc reports an unknown flag on one stream or the other; which does not
    // matter, only that it survived.
    assert.match(read("out.txt") + read("err.txt"), /nonsense/i);
  });
});
