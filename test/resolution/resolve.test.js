// Every question the store exists to answer, as assertions node itself has to
// satisfy. The hand-rolled harness this replaced collected results into an
// array and printed them, so six checks reported as one.
const { test, describe } = require("node:test");
const assert = require("node:assert/strict");
const path = require("node:path");

const dirOf = (pkg) => path.dirname(require.resolve(`${pkg}/package.json`));
const versionSeenFrom = (dir, pkg) => {
  const p = require.resolve(pkg, { paths: [dir] });
  return require(path.join(path.dirname(p), "package.json")).version;
};

describe("resolving through a store Please built", () => {
  test("a leaf package, reached through its top-level link", () => {
    assert.equal(typeof require("react").createElement, "function");
  });

  test("a package reaching its own dependencies", () => {
    // react-dom finds its scheduler and react through the symlinks inside its
    // own store entry, not through the top-level links.
    assert.equal(typeof require("react-dom").createPortal, "function");
  });

  test("a subpath export", () => {
    // Where resolution stops being a directory walk and starts consulting the
    // exports map.
    assert.equal(typeof require("react-dom/server").renderToString, "function");
  });

  test("a transitive dependency two levels down", () => {
    // react-dom -> loose-envify -> js-tokens
    assert.equal(versionSeenFrom(dirOf("loose-envify"), "js-tokens").split(".")[0], "4");
  });

  test("a package whose tarball ignores the package/ convention", () => {
    // DefinitelyTyped ships @types/node rooted at "node v22.18", space and all,
    // so assuming the convention silently produced an empty package.
    assert.equal(require("@types/node/package.json").name, "@types/node");
  });
});

// The one the store exists for: two packages, two different majors of one
// dependency, each resolving its own.
test("two versions of one dependency coexist", () => {
  const newSees = versionSeenFrom(dirOf("loose-envify"), "js-tokens");
  const oldSees = versionSeenFrom(dirOf("loose-envify-old"), "js-tokens");
  assert.equal(newSees.split(".")[0], "4", `new loose-envify saw ${newSees}`);
  assert.equal(oldSees.split(".")[0], "3", `old loose-envify saw ${oldSees}`);
});
