package esmdev

import (
	"github.com/evanw/esbuild/pkg/api"

	"tools/please_js/common"
)

// cssModuleTemplate wraps CSS content in a JS module that injects a <style> tag.
// Uses data-file attribute for identification so HMR can replace existing styles.
const cssModuleTemplate = `const __file = %q;
let s = document.querySelector('style[data-file="' + __file + '"]');
if (!s) { s = document.createElement('style'); s.dataset.file = __file; document.head.appendChild(s); }
s.textContent = %s;
`

// assetModuleTemplate wraps an asset URL in a JS module for ESM imports.
const assetModuleTemplate = `export default %q;
`

// assetExts is the set of file extensions treated as static assets.
var assetExts = func() map[string]bool {
	m := make(map[string]bool)
	for ext, loader := range common.Loaders {
		if loader == api.LoaderFile {
			m[ext] = true
		}
	}
	return m
}()

// isAssetExt reports whether the extension is a known asset type.
func isAssetExt(ext string) bool {
	return assetExts[ext]
}
