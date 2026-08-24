// Every question M1 exists to answer, as assertions node itself has to satisfy.
const assert = require("node:assert/strict");
const path = require("node:path");

const results = [];
function check(name, fn) {
    try {
        const detail = fn();
        results.push(["ok  ", name, detail ?? ""]);
    } catch (err) {
        results.push(["FAIL", name, err.message]);
        process.exitCode = 1;
    }
}

// 1. Baseline: a package with no dependencies, reached through the top-level link.
check("require a leaf package", () => {
    const react = require("react");
    assert.equal(typeof react.createElement, "function");
    return `react ${require("react/package.json").version}`;
});

// 2. The dep edge: react-dom must reach its own scheduler and react through the
//    symlinks inside its store entry, not through the top-level links.
check("require a package with deps", () => {
    const reactDom = require("react-dom");
    assert.equal(typeof reactDom.createPortal, "function");
    return `react-dom ${require("react-dom/package.json").version}`;
});

// 3. Subpath exports: where resolution stops being a directory walk and starts
//    consulting the "exports" map in package.json.
check("subpath export", () => {
    const server = require("react-dom/server");
    assert.equal(typeof server.renderToString, "function");
    return "react-dom/server";
});

// 4. A transitive dep two levels down: react-dom -> loose-envify -> js-tokens.
check("transitive dep resolves", () => {
    const p = require.resolve("js-tokens", {
        paths: [path.dirname(require.resolve("loose-envify/package.json"))],
    });
    const version = require(path.join(path.dirname(p), "package.json")).version;
    assert.equal(version.split(".")[0], "4");
    return `loose-envify sees js-tokens ${version}`;
});

// 5. THE ONE THAT MATTERS. Two packages, two different majors of the same
//    dependency, each resolving its own. This is what the store key exists for.
check("two versions of one dep coexist", () => {
    const newDir = path.dirname(require.resolve("loose-envify/package.json"));
    const oldDir = path.dirname(require.resolve("loose-envify-old/package.json"));

    const readVersion = (fromDir) => {
        const p = require.resolve("js-tokens", { paths: [fromDir] });
        return require(path.join(path.dirname(p), "package.json")).version;
    };

    const newSees = readVersion(newDir);
    const oldSees = readVersion(oldDir);
    assert.equal(newSees.split(".")[0], "4", `new loose-envify saw ${newSees}`);
    assert.equal(oldSees.split(".")[0], "3", `old loose-envify saw ${oldSees}`);
    assert.notEqual(newSees, oldSees);
    return `${newSees} and ${oldSees} side by side`;
});

for (const [status, name, detail] of results) {
    console.log(`${status}  ${name}${detail ? "  -- " + detail : ""}`);
}
console.log(process.exitCode ? "SPIKE FAILED" : "store resolves correctly");
