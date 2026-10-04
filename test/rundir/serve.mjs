// Prints site/index.html: a stand-in for a server reading a built site staged
// beside it.
import { readFileSync } from "node:fs";

process.stdout.write(readFileSync("site/index.html", "utf8"));
