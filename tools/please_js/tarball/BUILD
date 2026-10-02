subinclude("///go//build_defs:go")

go_library(
    name = "tarball",
    srcs = [
        "tarball.go",
        "verify.go",
    ],
    visibility = ["//tools/..."],
)

go_test(
    name = "tarball_test",
    srcs = ["tarball_test.go"],
    external = True,
    deps = [":tarball"],
)
