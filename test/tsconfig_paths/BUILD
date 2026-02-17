subinclude("//build_defs:js")

js_binary(
    name = "tsconfig_paths",
    entry_point = "src/main.ts",
    srcs = glob(["src/**/*.ts"]),
    tsconfig = "tsconfig.json",
    format = "cjs",
    platform = "node",
)
