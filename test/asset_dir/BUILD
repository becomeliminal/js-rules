subinclude("//build_defs:js")

js_binary(
    name = "asset_dir",
    entry_point = "src/main.ts",
    tsconfig = "tsconfig.json",
    assets = glob(["src/assets/**/*"]),
    format = "cjs",
    platform = "node",
)
