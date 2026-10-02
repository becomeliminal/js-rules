subinclude("///go//build_defs:go")

go_library(
    name = "store",
    srcs = [
        "dev.go",
        "linktree.go",
        "store.go",
    ],
    visibility = ["//tools/..."],
)

go_test(
    name = "store_test",
    srcs = [
        "linktree_test.go",
        "store_test.go",
    ],
    external = True,
    deps = [":store"],
)
