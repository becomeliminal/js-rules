subinclude("//build_defs:js")

js_binary(
    name = "integration",
    entry_point = "src/main.ts",
    srcs = glob(["src/**/*.ts"]),
    tsconfig = "tsconfig.json",
    format = "cjs",
    platform = "node",
    define = {
        "import.meta.env.VITE_API_BASE": '"https://api.example.com"',
        "import.meta.env.MODE": '"production"',
    },
    minify = True,
)
