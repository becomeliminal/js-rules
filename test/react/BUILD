subinclude("//build_defs:js")

# Server-side rendering test (node)
js_binary(
    name = "react",
    entry_point = "index.tsx",
    deps = [
        "//test/react/components",
        "//third_party/js:react",
        "//third_party/js:react-dom",
    ],
    format = "cjs",
    platform = "node",
)

# Browser bundle
js_binary(
    name = "app",
    entry_point = "app.tsx",
    deps = [
        "//test/react/components",
        "//third_party/js:react",
        "//third_party/js:react-dom",
    ],
    format = "iife",
    platform = "browser",
)

gentest(
    name = "react_test",
    test_cmd = " && ".join([
        "node test/react/react.js",
        "test -f test/react/app.js",
    ]),
    data = [":react", ":app"],
    no_test_output = True,
)
