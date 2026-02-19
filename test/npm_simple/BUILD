subinclude("//build_defs:js")

js_binary(
    name = "npm_simple",
    entry_point = "index.js",
    deps = ["//third_party/js:lodash"],
    format = "cjs",
    platform = "node",
)

gentest(
    name = "npm_simple_test",
    test_cmd = "node test/npm_simple/npm_simple.js",
    data = [":npm_simple"],
    no_test_output = True,
)
