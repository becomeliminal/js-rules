subinclude("//build_defs:js")

# A first-party library is named by where it lives, as a Go import path mirrors
# its directory: //test/library/greeter is imported as "test/library/greeter".
js_library(
    name = "greeter",
    srcs = ["index.js"],
    deps = ["//third_party/js:react_18.3.1"],
    visibility = ["//test/..."],
)
