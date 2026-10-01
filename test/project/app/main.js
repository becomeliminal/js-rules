const ms = require("ms");

// Resolved through the tree npm_project declared, from a package.json in this
// directory -- the whole surface an app writes to become an npm project.
const out = ms(7200000);
if (out !== "2h") {
  throw new Error(`resolved wrongly: ${out}`);
}
console.log("ok");
