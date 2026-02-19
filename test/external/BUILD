subinclude("//build_defs:js")

js_binary(
    name = "external",
    entry_point = "main.js",
    external = ["fs"],
    format = "cjs",
    platform = "node",
)

gentest(
    name = "external_test",
    test_cmd = "node test/external/external.js",
    data = [":external"],
    no_test_output = True,
)
