import { double, answer } from "@test/tool-lib";

if (answer() !== 42) {
  throw new Error("a data file the compiler never saw was not shipped");
}
if (double(21) !== 42) {
  throw new Error("first-party library returned the wrong value");
}
console.log("ok");
