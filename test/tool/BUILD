subinclude("//build_defs:npm", "//build_defs:js")

npm_link(
    name = "node_modules",
    store = [
        "//third_party/js:typescript",
        "//third_party/js:typescript_linux_x64",
    ],
    packages = {"typescript": "//third_party/js:typescript"},
)

# The keystone. tsc is an npm-provided binary that does its own module
# resolution to find its platform-specific compiler; running it as a build
# action is what every rule above this layer is a wrapper over. A type error
# here is a build failure.
js_run_binary(
    name = "compile",
    node_modules = ":node_modules",
    bin = "typescript/bin/tsc",
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
