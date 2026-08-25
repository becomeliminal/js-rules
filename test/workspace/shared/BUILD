subinclude("//build_defs:js")

# The repo's own package. Nothing about it says "workspace" -- it is an ordinary
# js_library, named the way its consumers import it.
js_library(
    name = "shared",
    package = "@test/shared",
    srcs = ["index.js"],
    visibility = ["//test/..."],
)

# The same package as a registry would receive it. npm_link takes either shape,
# so a package can be consumed inside the repo and published from it without
# being declared twice.
npm_package(
    name = "published",
    lib = ":shared",
    version = "2.0.0",
    visibility = ["//test/..."],
)
