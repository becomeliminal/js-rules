subinclude("//build_defs:js", "//build_defs:npm")

# Editing a third-party package without forking it. The patch changes what the
# package returns at runtime, so the assertion is on behaviour rather than on
# the file having changed.
npm_repo(
    name = "ms_2.1.3",
    hashes = ["f6616e15e530ed552f9daa2d3ce71963947c6bc7c98c9b64fd3e673fd02622c6"],
    patch = ["ms-plural.patch"],
    pkg = "ms",
    version = "2.1.3",
)

npm_link(
    name = "node_modules",
    lock = "pnpm-lock.yaml",
    packages = [":ms_2.1.3"],
)

js_test(
    name = "patch_test",
    entry_point = "main.js",
    node_modules = ":node_modules",
)
