const { greet } = require("test/library/greeter");
const out = greet("please");
if (!out.startsWith("hello please (react 18.3.1")) {
    console.error(`unexpected: ${out}`);
    process.exit(1);
}
console.log(`ok  ${out}`);
