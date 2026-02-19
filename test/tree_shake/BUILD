subinclude("//build_defs:js")

js_binary(
    name = "tree_shake",
    entry_point = "main.js",
    srcs = ["utils.js"],
    format = "cjs",
    platform = "node",
)

gentest(
    name = "tree_shake_test",
    test_cmd = "node test/tree_shake/tree_shake.js",
    data = [":tree_shake"],
    no_test_output = True,
)
