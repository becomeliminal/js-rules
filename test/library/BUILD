subinclude("//build_defs:js")

# A first-party library imported by name, resolving its own third-party
# dependency through the same tree.
js_test(
    name = "library_test",
    entry_point = "main.js",
    deps = ["//test/library/greeter"],
)
subinclude("//build_defs:js")
js_binary(name = "app", entry_point = "main.js", deps = ["//test/library/greeter"])
