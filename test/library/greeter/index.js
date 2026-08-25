// A first-party library, imported by name the way a Go package is imported by
// its path. Its own third-party dependency resolves through the same tree.
const React = require("react");
exports.greet = (who) => `hello ${who} (react ${React.version})`;
