import { strict as assert } from "node:assert";
import { greet } from "@utils/helpers";

assert.equal(greet("World"), "Hello, World!");
console.log("tsconfig_paths test passed");
