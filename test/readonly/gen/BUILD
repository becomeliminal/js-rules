# A source and a stale compiled copy of it, the copy read-only: the shape every
# input has under remote execution, which stages inputs read-only.
genrule(
    name = "inputs",
    outs = [
        "lib.js",
        "lib.ts",
    ],
    cmd = " && ".join([
        "echo 'export const answer: number = 42;' > lib.ts",
        "echo 'stale' > lib.js",
        "chmod 0444 lib.js",
    ]),
    visibility = ["//test/readonly/..."],
)
