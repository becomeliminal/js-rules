subinclude("//build_defs:js")

js_binary(
    name = "asset_import",
    entry_point = "main.js",
    srcs = ["logo.svg"],
    format = "cjs",
    platform = "node",
)

gentest(
    name = "asset_import_test",
    test_cmd = "node test/asset_import/asset_import.js",
    data = [":asset_import"],
    no_test_output = True,
)
