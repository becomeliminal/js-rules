import _ from "lodash";

const arr = [3, 1, 4, 1, 5, 9, 2, 6];
const sorted = _.sortBy(arr);
console.log("sorted:", sorted.join(", "));

const grouped = _.groupBy(["one", "two", "three"], "length");
console.log("grouped by length:", JSON.stringify(grouped));

const result = _.chunk(["a", "b", "c", "d", "e"], 2);
console.log("chunked:", JSON.stringify(result));

console.log("npm test passed");
