subinclude("//build_defs:js")

# The repo's own package. Nothing about it says "workspace" -- it is an ordinary
# js_library, named the way its consumers import it.
js_library(
    name = "shared",
    package = "@test/shared",
    srcs = ["index.js"],
    visibility = ["//test/..."],
)
