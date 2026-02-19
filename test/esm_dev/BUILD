subinclude("//build_defs:js")

js_dev_server(
    name = "dev",
    entry_point = "main.jsx",
    deps = [
        "//third_party/js:react",
        "//third_party/js:react-dom",
    ],
    servedir = ".",
    port = 3000,
    esm = True,
)

gentest(
    name = "prebundle_test",
    test_cmd = " && ".join([
        "cd test/esm_dev/_dev_prebundle",
        # A: No dynamic require calls
        '! grep -rn \'__require("[^"]*")\' deps/ --include="*.js" | grep -v "function __require()" | grep -q .',
        # B: Import map completeness
        """grep -roh 'from "[a-zA-Z@][^"]*"' deps/ --include='*.js' | sort -u | sed 's/from "//;s/"$//' | while read spec; do grep -q "\\"$spec\\"" importmap.json || { echo "FAIL: bare import '$spec' not in importmap"; exit 1; }; done""",
        # C: Expected entries
        """for pkg in react react-dom "react-dom/client" scheduler; do grep -q "\\"$pkg\\"" importmap.json || { echo "FAIL: missing importmap entry: $pkg"; exit 1; }; done""",
        # D: Import map targets exist on disk (skip prefix entries ending in /)
        """grep -o '"/@deps/[^"]*"' importmap.json | sed 's/"//g;s|/@deps/||' | grep -v '/$' | while read f; do test -f "deps/$f" || { echo "FAIL: importmap target missing: deps/$f"; exit 1; }; done""",
        # E: Prefix import map entries exist for each package
        """for pkg in react react-dom scheduler; do grep -q "\\"$pkg/\\"" importmap.json || { echo "FAIL: missing prefix entry: $pkg/"; exit 1; }; done""",
    ]),
    data = [":_dev_prebundle"],
    no_test_output = True,
)
