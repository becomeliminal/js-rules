subinclude("//build_defs:js", "//build_defs:npm")

npm_repo(
    name = "ms_2.1.3",
    hashes = ["f6616e15e530ed552f9daa2d3ce71963947c6bc7c98c9b64fd3e673fd02622c6"],
    pkg = "ms",
    version = "2.1.3",
)

# The tree the lockfile describes: one package fetched, one built here. They sit
# side by side and resolve identically, which is what lets a package move
# between the registry and the repo without an import changing.
npm_link(
    name = "node_modules",
    lock = "pnpm-lock.yaml",
    packages = [":ms_2.1.3"],
    workspace = {"@test/shared": "//test/workspace/shared:published"},
)

js_test(
    name = "workspace_test",
    entry_point = "main.js",
    node_modules = ":node_modules",
)
