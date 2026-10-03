subinclude("//build_defs:js")

# out_dirs is hoisted: the directory's contents are this target's outputs, at
# the package root, and the directory itself is not -- or every file is stored
# twice. The glob also yields src/sub itself, a directory entry beside the
# files in it, which staging has to take in its stride.
js_run_binary(
    name = "nested",
    srcs = glob(["src/**/*"]),
    args = [
        "src/a.ts",
        "src/sub/b.ts",
        "--outDir",
        "out",
        "--rootDir",
        "src",
    ],
    bin = "tsc",
    out_dirs = ["out"],
    package = "typescript",
)

# A directory output as a source is staged with everything in it, not dropped.
js_run_binary(
    name = "from_dir",
    srcs = ["//test/outdirs/gen:files"],
    outs = ["c.js"],
    args = [
        "gen/files/c.ts",
        "--outDir",
        ".",
        "--rootDir",
        "gen/files",
    ],
    bin = "tsc",
    package = "typescript",
)

js_test(
    name = "outdirs_test",
    srcs = ["outdirs.test.js"],
    data = [
        ":from_dir",
        ":nested",
    ],
)
