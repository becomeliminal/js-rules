import { strict as assert } from "node:assert";
import { getApiBase, getMode } from "@utils/config";

assert.equal(getApiBase(), "https://api.example.com");
assert.equal(getMode(), "production");
console.log("integration test passed");
