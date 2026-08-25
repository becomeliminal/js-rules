subinclude("//build_defs:js", "//build_defs:npm")

# ms ships no executable and declares none, so js_run_binary cannot run it --
# which is exactly the situation a package with an undeclared runnable file is
# in, and there is no small package on the registry that is genuinely in it.
npm_repo(
    name = "ms_2.1.3",
    bins = {"ms": "index.js"},
    hashes = ["f6616e15e530ed552f9daa2d3ce71963947c6bc7c98c9b64fd3e673fd02622c6"],
    pkg = "ms",
    version = "2.1.3",
)

npm_link(
    name = "node_modules",
    lock = "pnpm-lock.yaml",
    packages = [":ms_2.1.3"],
)

# Runs only because the executable was declared. Without bins, resolve-bin
# reports that the package publishes none and the build fails.
js_run_binary(
    name = "ran",
    package = "ms",
    node_modules = ":node_modules",
    exit_code_out = "status",
    stdout = "out.txt",
)

build_rule(
    name = "bins_test",
    srcs = ["check.sh"],
    data = [":ran"],
    outs = ["bins_test"],
    cmd = "cp $SRCS $OUT",
    binary = True,
    test = True,
    no_test_output = True,
    test_cmd = "$TEST",
)
