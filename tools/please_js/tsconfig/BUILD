subinclude("///go//build_defs:go")

go_library(
    name = "tsconfig",
    srcs = ["check.go"],
    visibility = ["//tools/..."],
)

go_test(
    name = "tsconfig_test",
    srcs = ["check_go_test.go"],
    deps = [":tsconfig"],
)
