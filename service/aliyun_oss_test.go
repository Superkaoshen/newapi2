package service

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestImageFileExtPrefersOriginalMimeExtension(t *testing.T) {
	tests := []struct {
		contentType string
		want        string
	}{
		{contentType: "image/jpeg", want: ".jpg"},
		{contentType: "image/jpg", want: ".jpg"},
		{contentType: "image/png", want: ".png"},
		{contentType: "image/webp", want: ".webp"},
	}

	for _, tt := range tests {
		if got := imageFileExt(tt.contentType); got != tt.want {
			t.Fatalf("imageFileExt(%q) = %q, want %q", tt.contentType, got, tt.want)
		}
	}
}

func TestBuildAliyunOssObjectKeyDoesNotUseJFIFForJPEG(t *testing.T) {
	key := buildAliyunOssObjectKey("test-prefix", "image/jpeg")
	if !strings.HasSuffix(key, ".jpg") {
		t.Fatalf("object key = %q, want .jpg suffix", key)
	}
	if strings.HasSuffix(key, ".jfif") {
		t.Fatalf("object key = %q, should not use .jfif suffix", key)
	}
}

func TestDetectImageContentTypeFromBytesPrefersActualImageFormat(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("DecodeString error = %v", err)
	}
	if got := detectImageContentTypeFromBytes(raw); got != "image/png" {
		t.Fatalf("detectImageContentTypeFromBytes = %q, want image/png", got)
	}
	if key := buildAliyunOssObjectKey("test-prefix", detectImageContentTypeFromBytes(raw)); !strings.HasSuffix(key, ".png") {
		t.Fatalf("object key = %q, want .png suffix", key)
	}
}
