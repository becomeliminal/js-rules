subinclude("///go//build_defs:go")

go_library(
    name = "store",
    srcs = [
        "dev.go",
        "store.go",
    ],
    visibility = ["//tools/..."],
)

go_test(
    name = "store_test",
    srcs = ["store_test.go"],
    external = True,
    deps = [":store"],
)
