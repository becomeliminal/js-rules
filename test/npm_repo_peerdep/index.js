// Tests that peerDependencies are included in the dep graph.
// react-dom has react as a peerDependency — without the peer dep fix,
// react-dom's generated BUILD file would not list react as a dep.
const React = require("react");
const ReactDOM = require("react-dom/server");

const { strict: assert } = require("node:assert");
const html = ReactDOM.renderToString(React.createElement("div", null, "peer dep test"));
assert.ok(html.includes("peer dep test"), "should render React element");
console.log("npm_repo_peerdep test passed");
