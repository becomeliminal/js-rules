// Browser platform test: Node builtins are replaced with empty CJS modules.
// Tests both side-effect imports AND named imports — packages like
// WalletConnect do `import { EventEmitter } from "events"` in dead code
// paths, and the empty stub must not fail at bundle time.

// Side-effect imports (every builtin, bare + node: prefix)
import "assert";
import "async_hooks";
import "child_process";
import "cluster";
import "constants";
import "crypto";
import "dgram";
import "diagnostics_channel";
import "dns";
import "domain";
import "fs";
import "http";
import "http2";
import "https";
import "inspector";
import "module";
import "net";
import "os";
import "path";
import "perf_hooks";
import "process";
import "punycode";
import "querystring";
import "readline";
import "repl";
import "stream";
import "string_decoder";
import "sys";
import "timers";
import "tls";
import "trace_events";
import "tty";
import "url";
import "util";
import "v8";
import "vm";
import "wasi";
import "worker_threads";
import "zlib";

import "node:assert";
import "node:async_hooks";
import "node:child_process";
import "node:cluster";
import "node:console";
import "node:constants";
import "node:crypto";
import "node:dgram";
import "node:diagnostics_channel";
import "node:dns";
import "node:domain";
import "node:fs";
import "node:http";
import "node:http2";
import "node:https";
import "node:inspector";
import "node:module";
import "node:net";
import "node:os";
import "node:path";
import "node:perf_hooks";
import "node:process";
import "node:punycode";
import "node:querystring";
import "node:readline";
import "node:repl";
import "node:stream";
import "node:string_decoder";
import "node:sys";
import "node:test";
import "node:timers";
import "node:trace_events";
import "node:tls";
import "node:tty";
import "node:url";
import "node:util";
import "node:v8";
import "node:vm";
import "node:wasi";
import "node:worker_threads";
import "node:zlib";

// Named imports — these must not fail at bundle time.
// The empty CJS stub (module.exports = {}) allows any named import
// since esbuild doesn't statically check CJS exports.
import { EventEmitter } from "events";
import { Buffer } from "buffer";
import { Readable, Writable } from "stream";
import { resolve, join } from "path";
import { readFileSync } from "fs";
import { createServer } from "http";
import { format } from "util";
import { Console } from "console";

// Default imports
import events from "events";
import buffer from "buffer";

console.log("node_builtins browser bundle test passed");
