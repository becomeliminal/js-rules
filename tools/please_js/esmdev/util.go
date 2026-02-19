package esmdev

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"

	"tools/please_js/common"
)

// transformEntry caches a transformed source file.
type transformEntry struct {
	code    []byte
	modTime time.Time
}

// isSourceFileExt returns true if the extension is a JS/TS source file.
func isSourceFileExt(ext string) bool {
	switch ext {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs":
		return true
	}
	return false
}

// resolveSourceFile finds the actual file for a URL path, trying various extensions.
func resolveSourceFile(sourceRoot, urlPath string) string {
	// Direct path
	full := filepath.Join(sourceRoot, filepath.FromSlash(urlPath))
	if info, err := os.Stat(full); err == nil && !info.IsDir() {
		return full
	}

	exts := []string{".ts", ".tsx", ".js", ".jsx"}

	// If the path has an extension like .js, try replacing it with .ts/.tsx/.jsx
	// This handles <script src="/main.js"> when the actual file is main.tsx
	if curExt := filepath.Ext(full); curExt != "" {
		base := strings.TrimSuffix(full, curExt)
		for _, ext := range exts {
			if ext == curExt {
				continue
			}
			candidate := base + ext
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}

	// Try adding extensions to the path as-is (for extensionless paths)
	for _, ext := range exts {
		candidate := full + ext
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	// Try index files
	for _, ext := range exts {
		candidate := filepath.Join(full, "index"+ext)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return ""
}

// loaderForFile returns the esbuild loader for a given file path.
func loaderForFile(path string) api.Loader {
	ext := filepath.Ext(path)
	if loader, ok := common.Loaders[ext]; ok {
		return loader
	}
	return api.LoaderJS
}

// getLocalIPs returns non-loopback IPv4 addresses.
func getLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ips = append(ips, ipnet.IP.String())
		}
	}
	return ips
}
