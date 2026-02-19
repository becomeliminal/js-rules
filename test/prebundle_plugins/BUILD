go_binary(
    name = "prebundle_plugins",
    srcs = ["main.go"],
    deps = [
        "//third_party/go:esbuild_api",
        "//tools/please_js/common",
    ],
)

gentest(
    name = "prebundle_plugins_test",
    test_cmd = "test/prebundle_plugins/prebundle_plugins",
    data = [
        ":prebundle_plugins",
        "fixtures/known-pkg/package.json",
        "fixtures/known-pkg/index.js",
        "fixtures/known-pkg/subpath.js",
        "fixtures/bad-pkg/package.json",
        "fixtures/bad-pkg/index.js",
        "fixtures/bad-pkg/lib.js",
    ],
    no_test_output = True,
)
