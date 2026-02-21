subinclude("//build_defs:js")

js_dev_server(
    name = "dev",
    entry_point = "main.tsx",
    srcs = glob(["**/*.tsx", "**/*.ts"]),
    deps = [
        "//third_party/js:react",
        "//third_party/js:react-dom",
    ],
    servedir = ".",
    port = 3102,
    esm = True,
)

gentest(
    name = "barrel_test",
    test_cmd = " && ".join([
        "cd test/esm_dev_barrel/_dev_prebundle",
        # A: Prebundle succeeded — deps dir exists
        'test -d deps || { echo "FAIL: deps dir missing"; exit 1; }',
        # B: Import map exists and has react entries
        'test -f importmap.json || { echo "FAIL: importmap.json missing"; exit 1; }',
        'grep -q "\"react\"" importmap.json || { echo "FAIL: react not in importmap"; exit 1; }',
    ]),
    data = [":_dev_prebundle"],
    no_test_output = True,
)
