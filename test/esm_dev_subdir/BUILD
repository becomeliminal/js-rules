subinclude("//build_defs:js")

js_dev_server(
    name = "dev",
    entry_point = "src/main.tsx",
    srcs = glob(["src/**/*.tsx", "src/**/*.css"]),
    deps = [
        "//third_party/js:react",
        "//third_party/js:react-dom",
        "//third_party/js:react-refresh",
    ],
    servedir = "public",
    port = 3001,
    esm = True,
)
