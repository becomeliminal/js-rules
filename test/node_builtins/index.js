// This file imports Node.js built-in modules that should be externalized
// in browser builds. If externalization is working, esbuild will treat
// these as external and the bundle will succeed despite platform: browser.
import "events";
import "util";
import "stream";
import "zlib";
import "assert";
import "buffer";
import "node:crypto";
import "node:path";

console.log("node_builtins browser bundle test passed");
