subinclude("//build_defs:js")

js_dev_server(
    name = "dev",
    entry_point = "app.jsx",
    deps = [
        "//third_party/js:react",
        "//third_party/js:react-dom",
    ],
    servedir = ".",
    port = 9999,
    format = "esm",
    platform = "browser",
)
