# Node resolving a real package graph through a store Please built: leaf
# packages, dep edges, subpath exports, a transitive dep, and two versions of
# one dependency side by side -- one of them bound under a second name.
build_rule(
    name = "resolve_test",
    srcs = ["run.sh"],
    data = [
        "resolve_test.js",
        "//third_party/js:node_modules",
        "//third_party/node:node",
    ],
    outs = ["resolve_test"],
    cmd = "cp $SRCS $OUT",
    binary = True,
    test = True,
    no_test_output = True,
    test_cmd = "$TEST",
)
