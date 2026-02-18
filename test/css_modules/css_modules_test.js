const { strict: assert } = require("node:assert");
const styles = require("./styles.module.css");

// CSS modules should export an object with scoped class names
assert.equal(typeof styles.button, "string");
assert.equal(typeof styles.title, "string");

// Scoped names should differ from originals
assert.notEqual(styles.button, "button");
assert.notEqual(styles.title, "title");

console.log("css modules test passed:", styles);
