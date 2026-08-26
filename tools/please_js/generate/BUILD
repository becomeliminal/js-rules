subinclude("///go//build_defs:go")

go_library(
    name = "generate",
    srcs = [
        "generate.go",
        "platform.go",
        "hash.go",
        "stray.go",
        "write.go",
    ],
    visibility = ["//tools/..."],
    deps = [
        "//third_party/go:buildtools",
        "//tools/please_js/lockfile",
        "//tools/please_js/store",
    ],
)

go_test(
    name = "generate_test",
    srcs = [
        "generate_test.go",
        "platform_test.go",
    ],
    external = True,
    deps = [
        ":generate",
        "//tools/please_js/lockfile",
    ],
)
