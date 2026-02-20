subinclude("//build_defs:js")

js_dev_server(
    name = "dev",
    entry_point = "main.js",
    deps = [
        "//third_party/js:events",
    ],
    servedir = ".",
    port = 3101,
    esm = True,
)

gentest(
    name = "events_polyfill_test",
    test_cmd = " && ".join([
        "cd test/esm_dev_events/_dev_prebundle",
        # A: events package is prebundled
        'test -f deps/events.js || { echo "FAIL: events.js not prebundled"; exit 1; }',
        # B: EventEmitter constructor exists (not an empty stub)
        'grep -q "EventEmitter" deps/events.js'
            ' || { echo "FAIL: EventEmitter not found in prebundled events"; '
            'echo "--- raw output ---"; cat deps/events.js; exit 1; }',
        # C: events is in the import map
        'grep -q "\"events\"" importmap.json'
            ' || { echo "FAIL: events not in importmap"; exit 1; }',
    ]),
    data = [":_dev_prebundle"],
    no_test_output = True,
)
