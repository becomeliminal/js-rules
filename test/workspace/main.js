const { shared } = require("@test/shared");
const ms = require("ms");

// One import from the repo, one from the registry, and the source cannot tell
// which is which -- that is the whole property.
const out = `${shared()} in ${ms(60000)}`;
if (out !== "from the workspace in 1m") {
  throw new Error(`resolved wrongly: ${out}`);
}
console.log("ok");
