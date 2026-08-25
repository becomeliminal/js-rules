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

# The entry point may be a file another target produced, which is what a
# ts_binary needs: point a binary at compiled output rather than at a source.
genrule(
    name = "generated_entry",
    outs = ["generated_main.js"],
    cmd = "echo 'console.log(\"ok  ran a generated entry point\")' > $OUT",
)

js_test(
    name = "generated_entry_test",
    entry_point = ":generated_entry",
)
