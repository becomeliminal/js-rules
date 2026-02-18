subinclude("//build_defs:js")

js_binary(
    name = "code_split",
    entry_point = "main.js",
    srcs = ["lazy.js", "shared.js"],
    splitting = True,
    html = True,
)

gentest(
    name = "code_split_test",
    test_cmd = " && ".join([
        # Entry chunk exists
        "test -f test/code_split/code_split/main.js",
        # At least 2 JS files produced (entry + lazy chunk)
        "test $(find test/code_split/code_split -name '*.js' | wc -l) -ge 2",
        # index.html exists and contains module script tag
        "test -f test/code_split/code_split/index.html",
        'grep -q \'type="module"\' test/code_split/code_split/index.html',
        # Entry runs correctly with Node
        'cd test/code_split/code_split && printf \'{"type":"module"}\' > package.json && node main.js | grep -q "hello world"',
    ]),
    data = [":code_split"],
    no_test_output = True,
)
