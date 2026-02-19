subinclude("//build_defs:js")

# Browser platform should use the "browser" field entry point
js_binary(
    name = "browser_field",
    entry_point = "browser_entry.js",
    platform = "browser",
    format = "esm",
    deps = ["//test/browser_field/fake_pkg"],
)

# Node platform should use the "main" field entry point
js_binary(
    name = "browser_field_node",
    entry_point = "node_entry.js",
    platform = "node",
    format = "cjs",
    deps = ["//test/browser_field/fake_pkg"],
)

gentest(
    name = "browser_field_test",
    test_cmd = " && ".join([
        "test -f test/browser_field/browser_field.js",
        "node test/browser_field/browser_field_node.js",
    ]),
    data = [":browser_field", ":browser_field_node"],
    no_test_output = True,
)
