subinclude("//build_defs:js")

js_binary(
    name = "css_import",
    entry_point = "main.js",
    srcs = ["style.css"],
    format = "cjs",
    platform = "node",
)

gentest(
    name = "css_import_test",
    test_cmd = "node test/css_import/css_import.js",
    data = [":css_import"],
    no_test_output = True,
)
