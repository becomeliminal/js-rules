subinclude("//build_defs:js")

js_binary(
    name = "tsconfig_paths",
    entry_point = "src/main.ts",
    srcs = glob(["src/**/*.ts"]),
    tsconfig = "tsconfig.json",
    format = "cjs",
    platform = "node",
)

gentest(
    name = "tsconfig_paths_test",
    test_cmd = "node test/tsconfig_paths/tsconfig_paths.js",
    data = [":tsconfig_paths"],
    no_test_output = True,
)
