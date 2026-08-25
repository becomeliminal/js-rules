subinclude("//build_defs:js")

# The keystone. tsc is an npm-provided binary that does its own module
# resolution to find its platform-specific compiler; running it as a build
# action is what every rule above this layer is a wrapper over. A type error
# here is a build failure.
js_run_binary(
    name = "compile",
    package = "typescript",
    bin = "tsc",
    srcs = ["hello.ts"],
    outs = ["hello.js"],
    args = ["hello.ts", "--target", "es2022", "--module", "esnext"],
)

# The compiled artifact is real JavaScript, not just a file that appeared.
build_rule(
    name = "compile_test",
    srcs = ["run_compiled.sh"],
    data = [
        ":compile",
        "//third_party/node:node",
    ],
    outs = ["compile_test"],
    cmd = "cp $SRCS $OUT",
    binary = True,
    test = True,
    no_test_output = True,
    test_cmd = "$TEST",
)

# The same keystone, over first-party code. tsc can only compile this if the
# library reached the tree it resolves through, and can only type-check it if it
# found the declarations -- so a successful build is the assertion. Without deps
# on js_run_binary the tool sees nothing but the lockfile, and this fails with
# "Cannot find module '@test/tool-lib'".
js_run_binary(
    name = "compile_lib",
    package = "typescript",
    bin = "tsc",
    srcs = ["uses_lib.ts"],
    outs = ["uses_lib.js"],
    deps = ["//test/tool/lib"],
    # tsc ignores this; the shell does not. Unquoted it splits, the word after
    # the space is taken as the command to run, and the build fails -- so a
    # build that succeeds is the assertion.
    env = {"TS_UNUSED": "a value with spaces"},
    args = [
        "uses_lib.ts",
        "--target",
        "es2022",
        "--module",
        "commonjs",
    ],
)

# And it still resolves at runtime, through the tree js_test assembles rather
# than the one the compiler saw.
js_test(
    name = "uses_lib_test",
    entry_point = ":compile_lib",
    deps = ["//test/tool/lib"],
)

