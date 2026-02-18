import { platform } from "fake-pkg";

if (platform !== "node") {
  throw new Error("expected node entry but got: " + platform);
}
console.log("browser_field node test passed");
