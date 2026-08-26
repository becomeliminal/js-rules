const ms = require("ms");

const out = ms(120000, { long: true });
if (out !== "2 minutes [patched]") {
  throw new Error(`the patch did not reach the running package: ${out}`);
}
console.log("ok");
