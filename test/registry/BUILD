subinclude("//build_defs:js", "//build_defs:npm")

# The exact-URL path, end to end: this is what the generator emits for every
# package in an npm lockfile, and for anything a private registry serves. The
# URL here is public, because the wiring is what is under test and a token
# would prove nothing a header test has not.
npm_repo(
    name = "ms_2.1.3",
    hashes = ["f6616e15e530ed552f9daa2d3ce71963947c6bc7c98c9b64fd3e673fd02622c6"],
    pkg = "ms",
    url = "https://registry.npmjs.org/ms/-/ms-2.1.3.tgz",
    version = "2.1.3",
)

js_test(
    name = "registry_test",
    srcs = ["registry.test.js"],
    node_modules = ":node_modules",
)

npm_link(
    name = "node_modules",
    lock = "pnpm-lock.yaml",
    packages = [":ms_2.1.3"],
)
