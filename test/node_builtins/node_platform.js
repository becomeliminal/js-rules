// Node platform test: builtins are externalized (real require/import at runtime).
// This verifies that node platform builds DON'T use the empty module plugin,
// so builtins resolve to real Node.js modules.
import path from "node:path";
import url from "node:url";
import util from "node:util";
import buffer from "node:buffer";
import stream from "node:stream";

// Actually use the builtins to prove they're real, not empty stubs
const joined = path.join("foo", "bar");
if (joined !== "foo/bar") throw new Error("path.join failed: " + joined);

const parsed = new url.URL("https://example.com/test");
if (parsed.pathname !== "/test") throw new Error("url.URL failed: " + parsed.pathname);

const formatted = util.format("hello %s", "world");
if (formatted !== "hello world") throw new Error("util.format failed: " + formatted);

if (typeof buffer.Buffer !== "function") throw new Error("buffer.Buffer is not a function");

if (typeof stream.Readable !== "function") throw new Error("stream.Readable is not a function");

console.log("node_builtins node platform test passed");
