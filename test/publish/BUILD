subinclude("//build_defs:js", "//build_defs:npm")

js_library(
    name = "lib",
    package = "@test/publishable",
    srcs = ["index.js"],
)

# What a registry gets. The name, entry point and exports map come from the
# library unchanged -- only what publishing needs is added.
npm_package(
    name = "package",
    lib = ":lib",
    version = "1.4.2",
    files = ["README.md"],
    fields = {
        "license": "Apache-2.0",
        "repository": '{"type":"git","url":"https://github.com/becomeliminal/js-rules"}',
    },
)

# The built package staged as a consumer would receive it, which is what makes
# the import assertion below a real one rather than a file check.
npm_link(
    name = "consumer",
    lock = "pnpm-lock.yaml",
    packages = [],
    workspace = {"@test/publishable": ":package"},
)

js_test(
    name = "publish_test",
    srcs = ["publish.test.js"],
    node_modules = ":consumer",
)

# A prerelease that names its commit. The stamp is as fresh as the last rebuild
# of this target, which is why it lives on a terminal target like this one.
npm_package(
    name = "stamped",
    lib = ":lib",
    stamp = True,
    version = "0.0.0-{describe}",
)

js_test(
    name = "stamped_test",
    srcs = ["stamped.test.js"],
    data = [":stamped"],
)
