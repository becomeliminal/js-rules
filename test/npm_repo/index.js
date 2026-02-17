import _ from "lodash";
import ms from "ms";

const arr = [3, 1, 4, 1, 5];
console.log("sorted:", _.sortBy(arr).join(", "));
console.log("1 hour =", ms("1h"), "ms");
console.log("npm_repo test passed");
