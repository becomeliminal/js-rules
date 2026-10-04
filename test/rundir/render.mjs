// Renders src/input.txt into out/rendered.txt: a stand-in for a tool that reads
// the package's sources and writes output beside them.
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import ms from "ms";

const input = readFileSync("src/input.txt", "utf8").trim();
const { suffix } = JSON.parse(readFileSync("config.json", "utf8"));
mkdirSync("out", { recursive: true });
writeFileSync("out/rendered.txt", `${input} ${ms(60000)} ${suffix}\n`);
console.log(`rendered from ${process.cwd()}`);
