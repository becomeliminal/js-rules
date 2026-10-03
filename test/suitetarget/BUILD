subinclude("//build_defs:js")

# Tests another rule produced, as compiled TypeScript would be: a directory of
# them, with a helper they import by relative path and a nested ESM test.
# fixture.js fails if it is ever run, and is not a *.test file, so the suite
# passing means only the test files in the directory ran.
genrule(
    name = "built_tests",
    srcs = [
        "helper.js",
        "fixture.js",
        "nested.test.mjs",
        "relative.test.js",
    ],
    outs = ["built"],
    cmd = " && ".join([
        "mkdir -p built/sub",
        "cp $PKG/helper.js $PKG/fixture.js $PKG/relative.test.js built/",
        "cp $PKG/nested.test.mjs built/sub/",
    ]),
    test_only = True,
)

# And a single built file, which is a test file whatever its name.
genrule(
    name = "built_test_file",
    srcs = ["relative.test.js"],
    outs = ["single_built.js"],
    cmd = "sed -e 's#./helper.js#./built/helper.js#' -e 's#a built test imports#a single built file imports#' $SRCS > $OUTS",
    test_only = True,
)

js_test(
    name = "suitetarget_test",
    srcs = [
        "plain.test.js",
        ":built_test_file",
        ":built_tests",
    ],
)
