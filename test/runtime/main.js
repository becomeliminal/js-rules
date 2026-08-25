const assert = (cond, msg) => { if (!cond) { console.error(msg); process.exit(1); } };

// Every argument the launcher is responsible for, checked from inside the
// program, because the launcher is generated shell and shell is where quoting
// goes wrong.
assert(process.env.GIVEN === "a value with $dollars and spaces",
  `env not passed through: ${JSON.stringify(process.env.GIVEN)}`);
assert(process.env.NOT_NAMED === undefined,
  "the whole environment leaked in, rather than what was named");

const args = process.argv.slice(2);
assert(args[0] === "--fixed" && args[1] === "with space",
  `fixed_args wrong: ${JSON.stringify(args)}`);

const cwd = process.cwd();
assert(cwd.endsWith("/test/runtime"), `chdir did not apply: ${cwd}`);

console.log("ok");
