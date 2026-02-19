subinclude("//build_defs:js")

js_dev_server(
    name = "dev",
    entry_point = "main.jsx",
    deps = [
        "//third_party/js:react",
        "//third_party/js:use-sync-external-store",
    ],
    servedir = ".",
    port = 3100,
    esm = True,
)

gentest(
    name = "cjs_fixup_test",
    test_cmd = " && ".join([
        "cd test/esm_dev_cjs/_dev_prebundle",
        # A: use-sync-external-store/shim/with-selector is prebundled
        'ls deps/use-sync-external-store/shim/ | grep -q "with-selector"'
            ' || { echo "FAIL: shim/with-selector not prebundled"; exit 1; }',
        # B: Named export exists (CJS fixup produced useSyncExternalStoreWithSelector)
        'grep -q "useSyncExternalStoreWithSelector" deps/use-sync-external-store/shim/with-selector*.js'
            ' || { echo "FAIL: named export useSyncExternalStoreWithSelector not found"; '
            'echo "--- raw output ---"; cat deps/use-sync-external-store/shim/with-selector*.js; exit 1; }',
        # C: No __require("react") remaining (should be replaced with static import)
        '! grep -q "__require" deps/use-sync-external-store/shim/with-selector*.js'
            ' || { echo "FAIL: __require still present"; '
            'grep "__require" deps/use-sync-external-store/shim/with-selector*.js; exit 1; }',
    ]),
    data = [":_dev_prebundle"],
    no_test_output = True,
)
