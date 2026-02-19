subinclude("//build_defs:js")

js_binary(
    name = "basic",
    entry_point = "index.js",
    deps = [
        "//test/basic/bar",
    ],
    platform = "node",
    format = "cjs",
)

gentest(
    name = "basic_test",
    test_cmd = "node test/basic/basic.js",
    data = [":basic"],
    no_test_output = True,
)
