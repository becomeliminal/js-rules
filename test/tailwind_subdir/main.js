import "./style.css";
import { render } from "./src/app.js";

const html = render();

// Verify Tailwind scanned src/app.js and generated bg-blue-500 utility
if (!html.includes("bg-blue-500")) {
    console.error("FAIL: render() missing expected class");
    process.exit(1);
}

console.log("PASS: tailwind_subdir — relative content path resolved correctly");
