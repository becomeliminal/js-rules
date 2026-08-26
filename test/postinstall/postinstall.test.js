const { test, describe } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");

const marker = () => JSON.parse(fs.readFileSync("test/postinstall/ms/pkg/marker.json", "utf8"));

describe("a command this repo chose", () => {
  test("runs inside the fetched package", () => {
    assert.ok(fs.existsSync("test/postinstall/ms/pkg/marker.json"),
      "postinstall did not run: no marker in the package");
  });

  test("finds node on PATH, as every install script assumes", () => {
    // The command is a node one-liner, so a marker at all proves it.
    assert.ok(marker());
  });

  test("receives the environment it was given, spaces intact", () => {
    assert.equal(marker().from, "a value with spaces");
  });

  test("receives nothing else", () => {
    // SHOULD_NOT_LEAK is exported around this build, so null is the claim: the
    // command sees what was named and not the machine it ran on.
    assert.equal(marker().leaked, null);
  });
});

describe("the package's own scripts", () => {
  test("run in npm's order rather than alphabetically", () => {
    const order = fs.readFileSync("test/postinstall/ms_with_hook/pkg/order", "utf8");
    assert.deepEqual(order.trim().split("\n"), ["first", "second", "third"]);
  });

  test("run after this repo's command, not before", () => {
    // Those scripts exist only because this repo's command wrote them, so their
    // having run at all is the ordering assertion.
    assert.ok(fs.existsSync("test/postinstall/ms_with_hook/pkg/order"));
  });

  test("do not run for a package that did not ask", () => {
    assert.ok(!fs.existsSync("test/postinstall/ms/pkg/order"),
      "hooks ran for a package that did not ask for them");
  });
});
