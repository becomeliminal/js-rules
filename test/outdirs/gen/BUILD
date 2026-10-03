# A source that is itself a directory: another rule's directory output.
genrule(
    name = "files",
    outs = ["files"],
    cmd = "mkdir files && echo 'export const c: string = \"c\";' > files/c.ts",
    visibility = ["//test/outdirs/..."],
)
