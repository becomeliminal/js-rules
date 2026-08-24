# A lockfile translated into targets, built, and resolved by node. The rules
# under test are generated, so they live in their own package: please_js update
# overwrites that file wholesale, as a generated file should.
build_rule(
    name = "dupes_test",
    srcs = ["run.sh"],
    data = [
        "dupes_test.js",
        "//test/generated:node_modules_lib-dupes",
        "//third_party/node:node",
    ],
    outs = ["dupes_test"],
    cmd = "cp $SRCS $OUT",
    binary = True,
    test = True,
    no_test_output = True,
    test_cmd = "$TEST",
)
