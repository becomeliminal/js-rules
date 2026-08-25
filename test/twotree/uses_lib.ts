import { double } from "@test/tool-lib";

if (double(21) !== 42) {
  throw new Error("first-party library returned the wrong value");
}
console.log("ok");
