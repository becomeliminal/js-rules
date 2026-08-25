subinclude("//build_defs:js")

js_library(
    name = "lib",
    package = "@test/publishable",
    srcs = ["index.js"],
)

# What a registry gets. The name, entry point and exports map come from the
# library unchanged -- only what publishing needs is added.
npm_package(
    name = "package",
    lib = ":lib",
    version = "1.4.2",
    files = ["README.md"],
    fields = {
        "license": "Apache-2.0",
        "repository": '{"type":"git","url":"https://github.com/becomeliminal/js-rules"}',
    },
)

build_rule(
    name = "publish_test",
    srcs = ["check.sh"],
    data = [
        ":package",
        "//third_party/node:node",
    ],
    outs = ["publish_test"],
    cmd = "cp $SRCS $OUT",
    binary = True,
    test = True,
    no_test_output = True,
    test_cmd = "$TEST",
)
