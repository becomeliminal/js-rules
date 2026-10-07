subinclude("//build_defs:js", "//build_defs:npm")

# A library that ships declarations: its published package must carry them,
# and the manifest's types entry must name a file that is there.
js_library(
    name = "lib",
    package = "@test/typed",
    srcs = ["index.js"],
    types = ["index.d.ts"],
)

npm_package(
    name = "package",
    lib = ":lib",
    version = "1.0.0",
)

js_test(
    name = "typed_test",
    srcs = ["typed.test.js"],
    data = [":package"],
)
