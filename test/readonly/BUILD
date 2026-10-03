subinclude("//build_defs:js")

# A tool rewriting a file that came in as a source: tsc compiles lib.ts over the
# read-only lib.js beside it. Staged files are made writable, as a local
# sandbox stages them, so this builds; without that it fails with EACCES, and
# it only ever failed remotely. vite copying public/ into its output and a
# later step rewriting one of those files is the same case.
js_run_binary(
    name = "overwrite",
    srcs = ["//test/readonly/gen:inputs"],
    args = [
        "gen/lib.ts",
        "--outDir",
        "gen",
    ],
    bin = "tsc",
    package = "typescript",
    stdout = "tsc.out",
)
