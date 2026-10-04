// An ES module entry: import syntax and top-level await, which only parse as a
// module. The test runs node with module detection off, so a copy that lost
// the .mjs type fails to parse here rather than being quietly reparsed.
import { setTimeout as sleep } from "node:timers/promises";

await sleep(1);
console.log("parsed as an ES module");
