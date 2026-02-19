package esmdev

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strings"
)

// parseProxies converts "prefix=target" strings into reverse proxy instances.
func parseProxies(specs []string) (map[string]*httputil.ReverseProxy, []string) {
	proxies := make(map[string]*httputil.ReverseProxy, len(specs))
	var prefixes []string
	for _, spec := range specs {
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 {
			continue
		}
		prefix := strings.TrimSpace(parts[0])
		target := strings.TrimSpace(parts[1])
		u, err := url.Parse(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: invalid proxy target %q: %v\n", target, err)
			continue
		}
		proxy := httputil.NewSingleHostReverseProxy(u)
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = u.Host
		}
		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		proxies[prefix] = proxy
		prefixes = append(prefixes, prefix)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})
	return proxies, prefixes
}
