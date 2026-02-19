subinclude("//build_defs:js")

tailwind_css(
    name = "tailwind",
    src = "input.css",
    content_srcs = ["page.html"],
    minify = False,
)

gentest(
    name = "tailwind_test",
    test_cmd = " && ".join([
        "test -f test/tailwind/tailwind.css",
        "test -s test/tailwind/tailwind.css",
    ]),
    data = [":tailwind"],
    no_test_output = True,
)
