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

# The package's own scripts, which is the other half. ms declares none, so this
# repo's command gives it one first -- which is exactly the ordering being
# asserted: a command written here runs before the package's scripts, the way
# npm applies patches before it runs them, because the command is usually what
# makes the script able to run at all.
#
# Contrived, and deliberately so. Every real package with an observable hook is
# hundreds of kilobytes and most only print a banner; this is 6KB and the effect
# is a file either present or absent.
npm_repo(
    name = "ms_with_hook",
    hashes = ["f6616e15e530ed552f9daa2d3ce71963947c6bc7c98c9b64fd3e673fd02622c6"],
    pkg = "ms",
    postinstall = "node -e 'const f=require(\"fs\"),m=JSON.parse(f.readFileSync(\"package.json\"));m.scripts={preinstall:\"echo first >> order\",postinstall:\"echo third >> order\",install:\"echo second >> order\"};f.writeFileSync(\"package.json\",JSON.stringify(m))'",
    run_hooks = True,
    version = "2.1.3",
)

js_test(
    name = "postinstall_test",
    srcs = ["postinstall.test.js"],
    data = [
        ":ms",
        ":ms_with_hook",
    ],
)
