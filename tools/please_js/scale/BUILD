subinclude("///go//build_defs:go")

go_test(
    name = "scale_test",
    srcs = ["scale_test.go"],
    external = True,
    # Generous against a slow CI machine, tight against a real regression.
    timeout = 300,
    deps = [
        "//tools/please_js/generate",
        "//tools/please_js/lockfile",
        "//tools/please_js/store",
    ],
)
