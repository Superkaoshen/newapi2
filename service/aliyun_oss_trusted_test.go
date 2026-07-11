package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateSameOriginURL(t *testing.T) {
	tests := []struct {
		name    string
		result  string
		base    string
		wantErr bool
	}{
		{name: "same private origin", result: "http://127.0.0.1:6001/generated/a.png", base: "http://127.0.0.1:6001", wantErr: false},
		{name: "relative resolved upstream", result: "http://127.0.0.1:6001/generated/a.png", base: "http://127.0.0.1:6001/v1", wantErr: false},
		{name: "different port", result: "http://127.0.0.1:6002/generated/a.png", base: "http://127.0.0.1:6001", wantErr: true},
		{name: "different scheme", result: "https://127.0.0.1:6001/generated/a.png", base: "http://127.0.0.1:6001", wantErr: true},
		{name: "userinfo lookalike", result: "http://127.0.0.1:6001@evil.example/generated/a.png", base: "http://127.0.0.1:6001", wantErr: true},
		{name: "same host userinfo", result: "http://user@127.0.0.1:6001/generated/a.png", base: "http://127.0.0.1:6001", wantErr: true},
		{name: "unsupported scheme", result: "ftp://127.0.0.1:6001/generated/a.png", base: "ftp://127.0.0.1:6001", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSameOriginURL(test.result, test.base)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateSameOriginURL() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestIsExplicitPrivateUpstreamHost(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{url: "http://127.0.0.1:6001", want: true},
		{url: "http://10.0.0.8:6001", want: true},
		{url: "http://[::1]:6001", want: true},
		{url: "https://example.com", want: false},
		{url: "http://localhost:6001", want: false},
		{url: "http://0.0.0.0:6001", want: false},
	}
	for _, test := range tests {
		if got := isExplicitPrivateUpstreamHost(test.url); got != test.want {
			t.Fatalf("isExplicitPrivateUpstreamHost(%q) = %v, want %v", test.url, got, test.want)
		}
	}
}

func TestDownloadTrustedURLBytesAllowsOnlySameOriginRasterRedirects(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	var crossOriginHits int
	crossOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		crossOriginHits++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer crossOrigin.Close()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect-same":
			http.Redirect(w, r, server.URL+"/image.png", http.StatusFound)
		case "/redirect-cross":
			http.Redirect(w, r, crossOrigin.URL+"/image.png", http.StatusFound)
		case "/fake.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("<html>not an image</html>"))
		default:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		}
	}))
	defer server.Close()

	data, contentType, err := downloadTrustedURLBytes(server.URL+"/redirect-same", true)
	if err != nil || string(data) != string(png) || contentType != "image/png" {
		t.Fatalf("same-origin raster redirect = (%d bytes, %q, %v)", len(data), contentType, err)
	}
	if _, _, err := downloadTrustedURLBytes(server.URL+"/redirect-cross", true); err == nil {
		t.Fatal("cross-origin redirect unexpectedly succeeded")
	}
	if crossOriginHits != 0 {
		t.Fatalf("cross-origin redirect target was requested %d times", crossOriginHits)
	}
	if _, _, err := downloadTrustedURLBytes(server.URL+"/fake.png", true); err == nil || !strings.Contains(err.Error(), "raster image") {
		t.Fatalf("fake image error = %v", err)
	}
}
