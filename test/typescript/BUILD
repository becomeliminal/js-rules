subinclude("//build_defs:js")

js_binary(
    name = "typescript",
    entry_point = "index.js",
    deps = ["//test/typescript/lib"],
    format = "cjs",
    platform = "node",
)

gentest(
    name = "typescript_test",
    test_cmd = "node test/typescript/typescript.js",
    data = [":typescript"],
    no_test_output = True,
)
