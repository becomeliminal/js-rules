subinclude("//build_defs:js")

npm_repo(
    name = "npm_deps",
    package_lock = "package-lock.json",
    subinclude_path = "@//build_defs:js",
)

_SUBREPO = "///test/esm_dev_regression/npm_deps//"

js_dev_server(
    name = "dev",
    entry_point = "main.tsx",
    deps = [
        # Core React
        _SUBREPO + "react",
        _SUBREPO + "react-dom",
        # Known regression packages
        _SUBREPO + "lodash-es",
        _SUBREPO + "use-sync-external-store",
        _SUBREPO + "events",
        _SUBREPO + "lowlight",
        # Simple ESM
        _SUBREPO + "clsx",
        _SUBREPO + "date-fns",
        _SUBREPO + "uuid",
        # Routing + data fetching
        _SUBREPO + "tanstack_react-query",
        _SUBREPO + "react-router-dom",
        # Tiptap ecosystem
        _SUBREPO + "tiptap_react",
        _SUBREPO + "tiptap_starter-kit",
        _SUBREPO + "tiptap_extension-code-block-lowlight",
        _SUBREPO + "tiptap_extension-character-count",
        _SUBREPO + "tiptap_extension-highlight",
        _SUBREPO + "tiptap_extension-image",
        _SUBREPO + "tiptap_extension-link",
        _SUBREPO + "tiptap_extension-placeholder",
        _SUBREPO + "tiptap_extension-subscript",
        _SUBREPO + "tiptap_extension-superscript",
        _SUBREPO + "tiptap_extension-table",
        _SUBREPO + "tiptap_extension-table-cell",
        _SUBREPO + "tiptap_extension-table-header",
        _SUBREPO + "tiptap_extension-table-row",
        _SUBREPO + "tiptap_extension-text-align",
        _SUBREPO + "tiptap_extension-typography",
        _SUBREPO + "tiptap_extension-underline",
        _SUBREPO + "tiptap_pm",
        _SUBREPO + "tiptap_suggestion",
        # Vercel
        _SUBREPO + "vercel_analytics",
        _SUBREPO + "vercel_speed-insights",
        # Solana
        _SUBREPO + "solana_spl-token",
        _SUBREPO + "solana_wallet-adapter-base",
        _SUBREPO + "solana_wallet-adapter-react",
        _SUBREPO + "solana_web3.js",
        # Charts
        _SUBREPO + "chart.js",
        _SUBREPO + "react-chartjs-2",
        # Web3
        _SUBREPO + "connectkit",
        _SUBREPO + "viem",
        _SUBREPO + "wagmi",
        # Icons
        _SUBREPO + "lucide-react",
        _SUBREPO + "react-icons",
        # Markdown
        _SUBREPO + "react-markdown",
        _SUBREPO + "remark-gfm",
        _SUBREPO + "remark-math",
        _SUBREPO + "rehype-katex",
        _SUBREPO + "tiptap-markdown",
        # Other
        _SUBREPO + "gsap",
        _SUBREPO + "zod",
        _SUBREPO + "openapi-fetch",
        _SUBREPO + "qrcode.react",
        _SUBREPO + "katex",
        _SUBREPO + "marked",
        _SUBREPO + "mermaid",
        # Crypto/encoding
        _SUBREPO + "bs58",
        _SUBREPO + "buffer",
        # Emotion (CSS-in-JS)
        _SUBREPO + "emotion_react",
        _SUBREPO + "emotion_styled",
        # Framer Motion
        _SUBREPO + "framer-motion",
        # State management
        _SUBREPO + "zustand",
        # Recharts (d3 subpaths)
        _SUBREPO + "recharts",
        # Forms
        _SUBREPO + "react-hook-form",
        _SUBREPO + "hookform_resolvers",
        # shadcn/UI deps
        _SUBREPO + "class-variance-authority",
        _SUBREPO + "tailwind-merge",
        # Radix UI primitives
        _SUBREPO + "radix-ui_react-dialog",
        _SUBREPO + "radix-ui_react-popover",
        _SUBREPO + "radix-ui_react-slot",
        _SUBREPO + "radix-ui_react-tooltip",
        _SUBREPO + "radix-ui_react-dropdown-menu",
        _SUBREPO + "radix-ui_react-select",
        _SUBREPO + "radix-ui_react-tabs",
        _SUBREPO + "radix-ui_react-accordion",
        # Command palette / toast / carousel / etc
        _SUBREPO + "cmdk",
        _SUBREPO + "sonner",
        _SUBREPO + "embla-carousel-react",
        _SUBREPO + "input-otp",
        _SUBREPO + "react-day-picker",
        _SUBREPO + "vaul",
        _SUBREPO + "tanstack_react-table",
    ],
    servedir = ".",
    port = 13200,
    esm = True,
)

sh_test(
    name = "regression_test",
    src = "test.sh",
    data = [":dev"],
    timeout = 120,
)
