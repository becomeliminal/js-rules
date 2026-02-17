import { used } from "./utils.js";
import { strict as assert } from "node:assert";

assert.equal(used(), "this function is used");
console.log("tree_shake test passed");
