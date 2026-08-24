subinclude("//build_defs:npm")

# Packages used by this repo's own tests. A consumer's third_party/js/BUILD has
# the same shape, generated from the lockfile.

npm_repo(
    name = "react",
    version = "18.3.1",
)

npm_repo(
    name = "scheduler",
    version = "0.23.2",
)

npm_repo(
    name = "js_tokens_4",
    pkg = "js-tokens",
    version = "4.0.0",
)

npm_repo(
    name = "js_tokens_3",
    pkg = "js-tokens",
    version = "3.0.2",
)

npm_repo(
    name = "loose_envify",
    pkg = "loose-envify",
    version = "1.4.0",
    deps = ["js_tokens_4"],
)

# Same package, different resolution, therefore a different store key. This is
# what lets two versions of js-tokens coexist.
npm_repo(
    name = "loose_envify_old",
    pkg = "loose-envify",
    version = "1.1.0",
    deps = ["js_tokens_3"],
)

npm_repo(
    name = "react_dom",
    pkg = "react-dom",
    version = "18.3.1",
    deps = ["loose_envify", "react", "scheduler"],
)

# TypeScript 7 is the Go-native compiler: a thin wrapper plus one platform
# binary package out of twenty. Exercises scoped packages and proves an
# npm-provided tool runs as a build action.
npm_repo(
    name = "typescript_linux_x64",
    pkg = "@typescript/typescript-linux-x64",
    version = "7.0.2",
)

npm_repo(
    name = "typescript",
    version = "7.0.2",
    deps = ["typescript_linux_x64"],
)
