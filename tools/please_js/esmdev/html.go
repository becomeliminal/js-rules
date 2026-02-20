package esmdev

import (
	"fmt"
	"regexp"
	"strings"
)

// liveReloadScript is injected into HTML pages for automatic reload on file changes.
// Used as fallback when react-refresh is not available.
const liveReloadScript = `<script type="module">
(() => {
  const es = new EventSource("/__esm_dev_sse");
  let t;
  es.addEventListener("change", () => {
    clearTimeout(t);
    t = setTimeout(() => location.reload(), 100);
  });
})();
</script>`

// refreshInitScript initializes react-refresh before React loads.
// Imports from "react-refresh" (main entry) which is guaranteed to be in the import map.
const refreshInitScript = `<script type="module">
import RefreshRuntime from "react-refresh";
RefreshRuntime.injectIntoGlobalHook(window);
window.$RefreshReg$ = () => {};
window.$RefreshSig$ = () => (type) => type;
window.__REACT_REFRESH__ = RefreshRuntime;
</script>`

// hmrClientScript is the HMR client that handles SSE events for hot module replacement.
const hmrClientScript = `<script type="module">
window.__ESM_HMR__ = {
  createContext(moduleUrl) {
    const hot = {
      _acceptCb: null,
      accept(cb) { hot._acceptCb = cb || (() => {}); },
    };
    window.__ESM_HMR__._modules.set(moduleUrl, hot);
    return hot;
  },
  _modules: new Map(),
};

const es = new EventSource("/__esm_dev_sse");

es.addEventListener("hmr-update", async (e) => {
  const { files } = JSON.parse(e.data);
  let didUpdate = false;
  for (const file of files) {
    try {
      await import(file + "?t=" + Date.now());
      didUpdate = true;
    } catch (err) {
      console.error("[hmr] Failed to update " + file, err);
      location.reload();
      return;
    }
  }
  if (didUpdate && window.__REACT_REFRESH__) {
    window.__REACT_REFRESH__.performReactRefresh();
  }
});

es.addEventListener("css-update", async (e) => {
  const { files } = JSON.parse(e.data);
  for (const file of files) {
    try {
      await import(file + "?t=" + Date.now());
    } catch (err) {
      console.warn("[hmr] CSS update failed for " + file, err);
    }
  }
});

es.addEventListener("full-reload", () => {
  location.reload();
});
</script>`

// HTML rewriting regexes for entry point resolution.
var (
	// Matches <script type="module" src="..."> to find module script tags.
	scriptSrcRe = regexp.MustCompile(`(<script\s[^>]*type=["']module["'][^>]*\ssrc=["'])([^"']+)(["'][^>]*>)`)
	// Matches <link rel="stylesheet" href="..."> to find CSS link tags.
	cssLinkRe = regexp.MustCompile(`<link\s[^>]*rel=["']stylesheet["'][^>]*href=["'][^"']+["'][^>]*/?>`)
	// Extracts href value from a link tag.
	hrefRe = regexp.MustCompile(`href=["']([^"']+)["']`)
)

// rewriteHTML transforms an HTML document for ESM dev serving:
// - Rewrites script src paths that don't resolve to the entry URL path
// - Removes CSS link tags that don't resolve (CSS is injected via JS modules)
// - Injects import map and client scripts before </head>
func rewriteHTML(html string, importMapJSON []byte, hasRefresh bool, entryURLPath, sourceRoot, packageRoot string) string {
	// Rewrite script src paths that don't resolve to real files.
	html = scriptSrcRe.ReplaceAllStringFunc(html, func(match string) string {
		parts := scriptSrcRe.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		src := parts[2]
		// Check if file exists in sourceRoot or packageRoot
		if resolveSourceFile(sourceRoot, src) != "" || resolveSourceFile(packageRoot, src) != "" {
			return match
		}
		// Replace with actual entry point path
		return parts[1] + entryURLPath + parts[3]
	})

	// Remove CSS link tags for files that don't exist — CSS is injected via JS modules.
	html = cssLinkRe.ReplaceAllStringFunc(html, func(match string) string {
		hrefMatch := hrefRe.FindStringSubmatch(match)
		if hrefMatch == nil {
			return match
		}
		href := hrefMatch[1]
		// Check if CSS file exists in sourceRoot or packageRoot
		if resolveSourceFile(sourceRoot, href) != "" || resolveSourceFile(packageRoot, href) != "" {
			return match
		}
		// Remove the tag — CSS is injected via JS modules in ESM dev mode
		return ""
	})

	// Safety net: if no module script tag points to the entry point, inject one.
	if !strings.Contains(html, `src="`+entryURLPath+`"`) && !strings.Contains(html, `src='`+entryURLPath+`'`) {
		entryScript := fmt.Sprintf(`<script type="module" src="%s"></script>`, entryURLPath)
		if idx := strings.Index(html, "</body>"); idx >= 0 {
			html = html[:idx] + entryScript + "\n" + html[idx:]
		} else {
			html = html + "\n" + entryScript
		}
	}

	// Inject import map and live reload / HMR script before </head>
	var clientScript string
	if hasRefresh {
		clientScript = refreshInitScript + "\n" + hmrClientScript
	} else {
		clientScript = liveReloadScript
	}
	injection := fmt.Sprintf(`<script type="importmap">%s</script>
%s`, string(importMapJSON), clientScript)

	if idx := strings.Index(html, "</head>"); idx >= 0 {
		html = html[:idx] + injection + "\n" + html[idx:]
	} else if idx := strings.Index(html, "<body"); idx >= 0 {
		html = html[:idx] + injection + "\n" + html[idx:]
	} else {
		html = injection + "\n" + html
	}

	return html
}
