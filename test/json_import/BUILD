subinclude("//build_defs:js")

js_binary(
    name = "json_import",
    entry_point = "main.js",
    srcs = ["data.json"],
    format = "cjs",
    platform = "node",
)

gentest(
    name = "json_import_test",
    test_cmd = "node test/json_import/json_import.js",
    data = [":json_import"],
    no_test_output = True,
)
