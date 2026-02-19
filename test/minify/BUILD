subinclude("//build_defs:js")

js_binary(
    name = "minify",
    entry_point = "main.js",
    format = "cjs",
    platform = "node",
    minify = True,
)

js_binary(
    name = "no_minify",
    entry_point = "main.js",
    format = "cjs",
    platform = "node",
)

gentest(
    name = "minify_test",
    test_cmd = " && ".join([
        "node test/minify/minify.js",
        "node test/minify/no_minify.js",
        "test $(wc -c < test/minify/minify.js) -lt $(wc -c < test/minify/no_minify.js)",
    ]),
    data = [":minify", ":no_minify"],
    no_test_output = True,
)
