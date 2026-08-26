subinclude("//build_defs:js")

# Node's own test runner, which needs nothing installed: it is part of the
# runtime node-rules pins. That is what lets a test of these rules avoid
# depending on the tree these rules build -- the same reason go_test uses
# stdlib testing.
#
# Please names each test, rather than reporting the file as one result.
# A file whose tests are not inside a describe -- which is most of them. Node
# writes those directly under <testsuites>, where Please does not look, so this
# file is what proves the conversion is doing something.
js_test(
    name = "greeter_test",
    srcs = [
        "flat.test.js",
        "greeter.test.js",
    ],
    deps = ["//test/library/greeter"],
)

# Both modes on one library, so the difference is visible: a suite reports each
# test, a program reports whether it exited as expected.
js_test(
    name = "rejects_test",
    entry_point = "rejects.js",
    expected_exit_code = 2,
    deps = ["//test/library/greeter"],
)
