package esmdev

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// sseEvent is sent to clients when files change.
type sseEvent struct {
	Type  string   `json:"type"`
	Files []string `json:"files,omitempty"`
}

// Component detection regexes for React Fast Refresh.
var (
	// function App(   or   export default function App(   or   export function App(
	funcComponentRe = regexp.MustCompile(`(?m)^(?:export\s+(?:default\s+)?)?function\s+([A-Z][a-zA-Z0-9_]*)\s*\(`)
	// const App =   or   export const App =   followed by arrow/function
	constComponentRe = regexp.MustCompile(`(?m)^(?:export\s+)?(?:const|let|var)\s+([A-Z][a-zA-Z0-9_]*)\s*=`)
)

// detectComponents returns the names of likely React components in transformed JS.
func detectComponents(code string) []string {
	seen := map[string]bool{}
	var names []string
	for _, m := range funcComponentRe.FindAllStringSubmatch(code, -1) {
		if !seen[m[1]] {
			names = append(names, m[1])
			seen[m[1]] = true
		}
	}
	for _, m := range constComponentRe.FindAllStringSubmatch(code, -1) {
		if !seen[m[1]] {
			names = append(names, m[1])
			seen[m[1]] = true
		}
	}
	return names
}

// injectRefreshRegistration wraps transformed JS code with React Fast Refresh
// registration calls for the given component names.
func injectRefreshRegistration(code []byte, urlPath string, components []string) []byte {
	var buf strings.Builder

	// Preamble: save/override global refresh hooks
	buf.WriteString("import.meta.hot = window.__ESM_HMR__?.createContext(")
	buf.WriteString(fmt.Sprintf("%q", urlPath))
	buf.WriteString(");\n")
	buf.WriteString("var __prevReg = window.$RefreshReg$;\n")
	buf.WriteString("var __prevSig = window.$RefreshSig$;\n")
	buf.WriteString("window.$RefreshReg$ = (type, id) => window.__REACT_REFRESH__?.register(type, ")
	buf.WriteString(fmt.Sprintf("%q", urlPath+" "))
	buf.WriteString(" + id);\n")
	buf.WriteString("window.$RefreshSig$ = window.__REACT_REFRESH__?.createSignatureFunctionForTransform || (() => (t) => t);\n")

	// Original code
	buf.Write(code)
	buf.WriteString("\n")

	// Footer: register components, then restore hooks
	for _, name := range components {
		buf.WriteString(fmt.Sprintf("window.$RefreshReg$(%s, %q);\n", name, name))
	}
	buf.WriteString("window.$RefreshReg$ = __prevReg;\n")
	buf.WriteString("window.$RefreshSig$ = __prevSig;\n")
	buf.WriteString("import.meta.hot?.accept();\n")

	return []byte(buf.String())
}

// broadcast sends an event to all connected SSE clients.
func (s *esmServer) broadcast(evt sseEvent) {
	s.sseMu.Lock()
	for ch := range s.clients {
		select {
		case ch <- evt:
		default:
		}
	}
	s.sseMu.Unlock()
}

// watchFiles polls the source tree for changes and broadcasts SSE events.
func (s *esmServer) watchFiles() {
	mtimes := make(map[string]time.Time)

	// Initial scan
	s.walkSourceTree(mtimes)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		newMtimes := make(map[string]time.Time)
		s.walkSourceTree(newMtimes)

		if !s.hasRefresh {
			// No HMR support — simple change detection with full reload
			changed := false
			for path, newMt := range newMtimes {
				if oldMt, ok := mtimes[path]; !ok || !oldMt.Equal(newMt) {
					changed = true
					s.transCache.Delete(path)
				}
			}
			for path := range mtimes {
				if _, ok := newMtimes[path]; !ok {
					changed = true
					s.transCache.Delete(path)
				}
			}
			if changed {
				s.clearTailwindCache()
				mtimes = newMtimes
				s.broadcast(sseEvent{Type: "change"})
			}
			continue
		}

		// HMR-aware change classification
		var hmrFiles []string
		var cssFiles []string
		needFullReload := false

		for path, newMt := range newMtimes {
			if oldMt, ok := mtimes[path]; !ok || !oldMt.Equal(newMt) {
				s.transCache.Delete(path)

				rel, err := filepath.Rel(s.packageRoot, path)
				var relPath string
				if err == nil && !strings.HasPrefix(rel, "..") {
					relPath = "/" + filepath.ToSlash(rel)
				} else {
					// Check if this file is in a local library directory
					relPath = s.libURLPath(path)
				}
				if relPath == "" {
					needFullReload = true
					continue
				}
				ext := filepath.Ext(path)

				switch {
				case ext == ".css":
					cssFiles = append(cssFiles, relPath)
				case relPath == s.entryURLPath:
					needFullReload = true
				case isSourceFileExt(ext):
					if isComp, ok := s.componentFiles.Load(path); ok && isComp.(bool) {
						hmrFiles = append(hmrFiles, relPath)
					} else {
						needFullReload = true
					}
				default:
					needFullReload = true
				}
			}
		}
		// Check for deleted files
		for path := range mtimes {
			if _, ok := newMtimes[path]; !ok {
				s.transCache.Delete(path)
				s.componentFiles.Delete(path)
				needFullReload = true
			}
		}

		if needFullReload {
			s.clearTailwindCache()
			mtimes = newMtimes
			s.broadcast(sseEvent{Type: "full-reload"})
		} else if len(hmrFiles) > 0 || len(cssFiles) > 0 {
			s.clearTailwindCache()
			mtimes = newMtimes
			if len(hmrFiles) > 0 {
				s.broadcast(sseEvent{Type: "hmr-update", Files: hmrFiles})
			}
			if len(cssFiles) > 0 {
				s.broadcast(sseEvent{Type: "css-update", Files: cssFiles})
			}
		}
	}
}

// walkSourceTree collects file mtimes, skipping hidden dirs, node_modules, plz-out.
// Walks packageRoot (which may be a parent of sourceRoot) and local library directories.
func (s *esmServer) walkSourceTree(mtimes map[string]time.Time) {
	walkDir := func(root string) {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if strings.HasPrefix(name, ".") || name == "node_modules" || name == "plz-out" {
					return filepath.SkipDir
				}
				return nil
			}

			ext := filepath.Ext(path)
			switch ext {
			case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".css", ".html", ".json":
				mtimes[path] = info.ModTime()
			}
			return nil
		})
	}

	walkDir(s.packageRoot)
	for _, libDir := range s.localLibs {
		// Skip if libDir is already under packageRoot (would be walked already)
		if strings.HasPrefix(libDir, s.packageRoot+"/") {
			continue
		}
		walkDir(libDir)
	}
}

// libURLPath returns the /@lib/ URL path for a file in a local library dir,
// or "" if the file doesn't belong to any library.
func (s *esmServer) libURLPath(absPath string) string {
	for name, dir := range s.localLibs {
		if strings.HasPrefix(absPath, dir+"/") {
			rel, _ := filepath.Rel(dir, absPath)
			return "/@lib/" + name + "/" + filepath.ToSlash(rel)
		}
	}
	return ""
}

// handleSSE handles Server-Sent Events connections for live reload and HMR.
func (s *esmServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher.Flush()

	ch := make(chan sseEvent, 1)
	s.sseMu.Lock()
	s.clients[ch] = struct{}{}
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.clients, ch)
		s.sseMu.Unlock()
	}()

	keepAlive := time.NewTicker(30 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-ch:
			data, _ := json.Marshal(evt)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()
		case <-keepAlive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
