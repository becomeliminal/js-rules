// A first-party library, imported by name the way a Go package is imported by
// its path. Its own third-party dependency resolves through the same tree.
const React = require("react");
exports.greet = (who) => `hello ${who} (react ${React.version})`;

// Deliberately untested from the suite fixture, so coverage has something to
// catch. The body has lines of its own, because a one-line arrow is "covered"
// the moment the module loads -- the assignment line executes -- which is a
// V8 line-coverage fact worth remembering when reading these numbers.
exports.unexercised = (who) => {
  const parts = ["goodbye", who];
  return parts.join(" ");
};
