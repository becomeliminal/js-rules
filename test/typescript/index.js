import { greet, greetAll } from "test/typescript/lib";

const result = greet({ name: "TypeScript", greeting: "Hi" });
console.log(result);

const all = greetAll(["Alice", "Bob", "Charlie"]);
all.forEach(g => console.log(g));

console.log("typescript test passed");
