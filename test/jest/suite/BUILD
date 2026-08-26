subinclude("//build_defs:js")

# A suite already written in jest, run under Please. The snapshot is committed
# beside the test, and --ci is what makes a mismatch fail rather than quietly
# rewriting it -- a test that edits its own expectations under a build system
# is not a test.
jest_test(
    name = "jest_test",
    srcs = [
        "__snapshots__/math.test.js.snap",
        "math.js",
        "math.test.js",
    ],
    node_modules = "//test/jest:node_modules",
)
