import debug from "debug";
import ms from "ms";

const log = debug("test");
log.enabled = true;
log("1 hour = %d ms", ms("1h"));
console.log("npm_repo_cycle test passed");
