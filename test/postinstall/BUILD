subinclude("//build_defs:js", "//build_defs:npm")

# A command this repo chose, run inside a fetched package. Deliberately not the
# package's own install script: ms has none, and running whatever a third party
# shipped is the thing worth never doing by default.
#
# One fixture, three properties. The command runs at all; node is on PATH, which
# every real install script assumes because npm puts it there; and the
# environment is the one declared here rather than the build's.
npm_repo(
    name = "ms",
    hashes = ["f6616e15e530ed552f9daa2d3ce71963947c6bc7c98c9b64fd3e673fd02622c6"],
    pkg = "ms",
    postinstall = "node -e 'require(\"fs\").writeFileSync(\"marker.json\", JSON.stringify({from: process.env.MARKER, leaked: process.env.SHOULD_NOT_LEAK || null}))'",
    postinstall_env = {"MARKER": "a value with spaces"},
    version = "2.1.3",
)

build_rule(
    name = "postinstall_test",
    srcs = ["check.sh"],
    data = [":ms"],
    outs = ["postinstall_test"],
    cmd = "cp $SRCS $OUT",
    binary = True,
    test = True,
    no_test_output = True,
    test_cmd = "$TEST",
)
