const fs = require("fs");
const path = require("path");
const ms = require("ms");
const { version } = require("ms/package.json");

// This project's package.json asks for ms 2.1.3, and the tree's other
// project asks for a different version: each must see only its own pin.
if (version !== "2.1.3") {
  throw new Error(`resolved ms ${version}, want 2.1.3`);
}
if (ms(7200000) !== "2h") {
  throw new Error("ms did not load");
}

// In the store layout -- where ms is a symlink into .plz -- nothing of the
// other project's is staged at all.
const nm = path.join(__dirname, "node_modules");
if (fs.lstatSync(path.join(nm, "ms")).isSymbolicLink()) {
  const entries = fs.readdirSync(path.join(nm, ".plz")).filter((e) => e.startsWith("ms_"));
  if (entries.length !== 1 || entries[0] !== "ms_2.1.3") {
    throw new Error(`store holds ${entries.join(", ")}, want only ms_2.1.3`);
  }
  console.log("store holds only ms_2.1.3");
}
console.log("ok");
