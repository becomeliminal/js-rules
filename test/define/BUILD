subinclude("//build_defs:js")

js_binary(
    name = "define",
    entry_point = "main.js",
    format = "cjs",
    platform = "node",
    define = {
        "API_BASE": '"https://api.example.com"',
        "import.meta.env.MODE": '"testing"',
    },
)
