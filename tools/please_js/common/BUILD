go_library(
    name = "common",
    srcs = ["common.go", "env.go", "package_json.go", "target.go"],
    deps = [
        "//third_party/go:esbuild_api",
    ],
    visibility = ["//tools/please_js/...", "//test/prebundle_plugins/..."],
)

go_test(
    name = "common_test",
    srcs = ["common_test.go", "package_json_test.go"],
    deps = [
        ":common",
        "//third_party/go:esbuild_api",
    ],
)
