subinclude("//build_defs:js")

# Ships declarations beside the JavaScript, so a tool that type-checks against
# it has something to read.
js_library(
    name = "lib",
    package = "@test/tool-lib",
    srcs = [
        "index.d.ts",
        "index.js",
    ],
    visibility = ["//test/..."],
)
