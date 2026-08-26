// A program, not a suite. Some things are only expressible as an exit status --
// a command line tool refusing bad input, a bundle that must run.
const { greet } = require("@test/greeter");

if (typeof greet !== "function") {
  process.exit(1);
}
process.exit(2);
