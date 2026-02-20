subinclude("//build_defs:js")

# Uses lodash-es — a pure ESM package with no "exports" field in package.json,
# where individual files use default exports. This is the package structure
# that triggered the "does not provide an export named 'default'" bug.
js_dev_server(
    name = "dev",
    entry_point = "main.js",
    deps = [
        "//third_party/js:lodash-es",
    ],
    servedir = ".",
    port = 3200,
    esm = True,
)

gentest(
    name = "lodash_es_test",
    test_cmd = " && ".join([
        "cd test/esm_dev_lodash_es/_dev_prebundle",
        # A: lodash-es main entry is prebundled
        'test -f deps/lodash-es.js || { echo "FAIL: lodash-es.js not prebundled"; exit 1; }',
        # B: lodash-es main entry has export statements
        'grep -q "export" deps/lodash-es.js'
            ' || { echo "FAIL: lodash-es.js has no exports"; exit 1; }',
        # C: Import map has lodash-es entry
        'grep -q "\"lodash-es\"" importmap.json'
            ' || { echo "FAIL: lodash-es not in importmap"; exit 1; }',
        # D: Import map has lodash-es/ prefix entry for subpath resolution
        'grep -q "\"lodash-es/\"" importmap.json'
            ' || { echo "FAIL: lodash-es/ prefix entry not in importmap"; exit 1; }',
        # E: lodash-es main entry contains memoize (bundled from lodash.js → memoize.js)
        'grep -q "memoize" deps/lodash-es.js'
            ' || { echo "FAIL: lodash-es.js does not contain memoize"; exit 1; }',
    ]),
    data = [":_dev_prebundle"],
    no_test_output = True,
)
