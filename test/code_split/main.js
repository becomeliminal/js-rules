import { GREETING } from "./shared.js";
async function main() {
    const { greet } = await import("./lazy.js");
    console.log(greet("world"));
    console.log(GREETING);
}
main();
