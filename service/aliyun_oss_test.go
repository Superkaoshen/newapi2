package service

import (
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
