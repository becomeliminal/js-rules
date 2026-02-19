subinclude("//build_defs:js")

js_binary(
    name = "asset_dir",
    entry_point = "src/main.ts",
    tsconfig = "tsconfig.json",
    assets = glob(["src/assets/**/*"]),
    format = "cjs",
    platform = "node",
)

gentest(
    name = "asset_dir_test",
    test_cmd = "node test/asset_dir/asset_dir.js",
    data = [":asset_dir"],
    no_test_output = True,
)
