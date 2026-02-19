package esmdev

import (
	"testing"
)

func TestParseProxies(t *testing.T) {
	t.Run("single proxy", func(t *testing.T) {
		proxies, prefixes := parseProxies([]string{"/api=http://localhost:8080"})

		if len(prefixes) != 1 || prefixes[0] != "/api" {
			t.Fatalf("expected prefixes [/api], got %v", prefixes)
		}
		if _, ok := proxies["/api"]; !ok {
			t.Error("expected proxy entry for /api")
		}
	})

	t.Run("multiple proxies sorted by length desc", func(t *testing.T) {
		proxies, prefixes := parseProxies([]string{
			"/api=http://localhost:8080",
			"/api/v2/admin=http://localhost:9090",
			"/api/v2=http://localhost:8081",
		})

		if len(prefixes) != 3 {
			t.Fatalf("expected 3 prefixes, got %d", len(prefixes))
		}
		if len(proxies) != 3 {
			t.Fatalf("expected 3 proxies, got %d", len(proxies))
		}
		// Longest prefix first
		if prefixes[0] != "/api/v2/admin" {
			t.Errorf("expected first prefix /api/v2/admin, got %s", prefixes[0])
		}
		if prefixes[1] != "/api/v2" {
			t.Errorf("expected second prefix /api/v2, got %s", prefixes[1])
		}
		if prefixes[2] != "/api" {
			t.Errorf("expected third prefix /api, got %s", prefixes[2])
		}
	})

	t.Run("invalid spec skipped", func(t *testing.T) {
		proxies, prefixes := parseProxies([]string{"no-equals-sign"})

		if len(prefixes) != 0 {
			t.Errorf("expected no prefixes, got %v", prefixes)
		}
		if len(proxies) != 0 {
			t.Errorf("expected no proxies, got %d", len(proxies))
		}
	})

	t.Run("empty specs", func(t *testing.T) {
		proxies, prefixes := parseProxies([]string{})

		if len(proxies) != 0 {
			t.Errorf("expected empty map, got %d entries", len(proxies))
		}
		if prefixes != nil {
			t.Errorf("expected nil slice, got %v", prefixes)
		}
	})
}
