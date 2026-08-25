subinclude("//build_defs:js")

# Ships declarations beside the JavaScript, so a tool that type-checks against
# it has something to read.
js_library(
    name = "lib",
    package = "@test/tool-lib",
    srcs = ["index.js"],
    # Declarations recorded apart from sources: a compiler reads them, a bundler
    # and node never do.
    types = ["index.d.ts"],
    # And a runtime file no compiler should see at all.
    data = ["table.json"],
    visibility = ["//test/..."],
)
