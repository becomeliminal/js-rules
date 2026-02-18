import { platform } from "fake-pkg";

if (platform !== "browser") {
  throw new Error("expected browser entry but got: " + platform);
}
console.log("browser_field test passed");
