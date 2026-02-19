go_library(
    name = "esmdev",
    srcs = glob(["*.go"], exclude = ["*_test.go"]),
    deps = [
        "//third_party/go:esbuild_api",
        "//third_party/go:x_sync",
        "//tools/please_js/common",
    ],
    visibility = ["//tools/please_js/..."],
)

go_test(
    name = "esmdev_test",
    srcs = glob(["*_test.go"]),
    deps = [":esmdev"],
)
