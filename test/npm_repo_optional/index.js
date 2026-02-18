const ms = require("ms");
const debug = require("debug");
const log = debug("test");
log("optional dep test: debug loaded");
console.log("optional dep test passed:", ms("1h"));
