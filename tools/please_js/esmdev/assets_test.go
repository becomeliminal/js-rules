package esmdev

import (
	"testing"
)

func TestIsAssetExt(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".png", true},
		{".jpg", true},
		{".svg", true},
		{".woff2", true},
		{".js", false},
		{".ts", false},
		{".css", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := isAssetExt(tt.ext)
			if got != tt.want {
				t.Errorf("isAssetExt(%q) = %v, want %v", tt.ext, got, tt.want)
			}
		})
	}
}

func TestIsTextExt(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".md", true},
		{".astro", true},
		{".js", false},
		{".ts", false},
		{".css", false},
		{".png", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := isTextExt(tt.ext)
			if got != tt.want {
				t.Errorf("isTextExt(%q) = %v, want %v", tt.ext, got, tt.want)
			}
		})
	}
}
