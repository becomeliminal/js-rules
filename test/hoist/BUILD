subinclude("//build_defs:js", "//build_defs:npm")

npm_repo(
    name = "debug_4.4.0",
    hashes = ["fd9c4f0ca9acd1103099f038754eb21f94497456fb9b195b18fd582fa6982d1c"],
    pkg = "debug",
    version = "4.4.0",
)

npm_repo(
    name = "ms_2.1.3",
    hashes = ["f6616e15e530ed552f9daa2d3ce71963947c6bc7c98c9b64fd3e673fd02622c6"],
    pkg = "ms",
    version = "2.1.3",
)

# The escape hatch, exercised. @test/broken imports ms without declaring it, and
# ms is transitive, so without the hoist the import fails at run time with
# Cannot find module -- the strict store being right. The hoist records the
# workaround where a reviewer sees it.
npm_link(
    name = "node_modules",
    hoist = ["ms"],
    lock = "pnpm-lock.yaml",
    packages = [
        ":debug_4.4.0",
        ":ms_2.1.3",
    ],
)

js_test(
    name = "hoist_test",
    srcs = ["hoist.test.js"],
    deps = ["//test/hoist/broken"],
    node_modules = ":node_modules",
)
