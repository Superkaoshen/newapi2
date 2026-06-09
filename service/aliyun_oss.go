package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/google/uuid"
)

type AliyunOssConfig struct {
	Enabled         bool
	Endpoint        string
	Bucket          string
	AccessKeyId     string
	AccessKeySecret string
	PathPrefix      string
	PublicBaseURL   string
}

var markdownImageURLRegex = regexp.MustCompile(`!\[[^\]]*]\(([^)\r\n]+)\)`)

func GetAliyunOssConfig() AliyunOssConfig {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()

	return AliyunOssConfig{
		Enabled:         common.OptionMap["AliyunOssEnabled"] == "true",
		Endpoint:        strings.TrimSpace(common.OptionMap["AliyunOssEndpoint"]),
		Bucket:          strings.TrimSpace(common.OptionMap["AliyunOssBucket"]),
		AccessKeyId:     strings.TrimSpace(common.OptionMap["AliyunOssAccessKeyId"]),
		AccessKeySecret: strings.TrimSpace(common.OptionMap["AliyunOssAccessKeySecret"]),
		PathPrefix:      strings.TrimSpace(common.OptionMap["AliyunOssPathPrefix"]),
		PublicBaseURL:   strings.TrimSpace(common.OptionMap["AliyunOssPublicBaseUrl"]),
	}
}

func (c AliyunOssConfig) IsEnabledAndValid() bool {
	return c.Enabled &&
		c.Endpoint != "" &&
		c.Bucket != "" &&
		c.AccessKeyId != "" &&
		c.AccessKeySecret != ""
}

func IsAliyunOssEnabled() bool {
	return GetAliyunOssConfig().IsEnabledAndValid()
}

func ReplaceMarkdownImageURLsWithAliyunOSS(content string, upstreamBaseURL string) (string, bool) {
	cfg := GetAliyunOssConfig()
	if !cfg.IsEnabledAndValid() {
		return content, false
	}
	if content == "" || !strings.Contains(content, "](") {
		return content, false
	}

	changed := false
	cache := make(map[string]string)
	replaced := markdownImageURLRegex.ReplaceAllStringFunc(content, func(match string) string {
		subMatches := markdownImageURLRegex.FindStringSubmatch(match)
		if len(subMatches) != 2 {
			return match
		}

		rawURL := strings.TrimSpace(subMatches[1])
		if rawURL == "" || strings.HasPrefix(rawURL, "data:") {
			return match
		}

		if cachedURL, ok := cache[rawURL]; ok {
			if cachedURL != rawURL {
				changed = true
				return strings.Replace(match, rawURL, cachedURL, 1)
			}
			return match
		}

		savedURL, err := SaveImageURLToAliyunOSS(rawURL, upstreamBaseURL)
		if err != nil {
			common.SysError(fmt.Sprintf("failed to save image url to aliyun oss: %s", err.Error()))
			cache[rawURL] = rawURL
			return match
		}

		cache[rawURL] = savedURL
		if savedURL == rawURL {
			return match
		}
		changed = true
		return strings.Replace(match, rawURL, savedURL, 1)
	})

	return replaced, changed
}

func SaveImageURLToAliyunOSS(rawURL string, upstreamBaseURL string) (string, error) {
	cfg := GetAliyunOssConfig()
	if !cfg.IsEnabledAndValid() {
		return rawURL, nil
	}

	resolvedURL, err := resolveImageURL(rawURL, upstreamBaseURL)
	if err != nil {
		return "", err
	}

	data, contentType, err := downloadImageBytes(resolvedURL)
	if err != nil {
		return "", err
	}

	objectKey := buildAliyunOssObjectKey(cfg.PathPrefix, contentType)
	if err := uploadBytesToAliyunOSS(cfg, objectKey, data, contentType); err != nil {
		return "", err
	}

	return buildAliyunOssPublicURL(cfg, objectKey)
}

func SaveFileURLToAliyunOSS(rawURL string, upstreamBaseURL string, fallbackContentType string) (string, error) {
	cfg := GetAliyunOssConfig()
	if !cfg.IsEnabledAndValid() {
		return rawURL, nil
	}

	resolvedURL, err := resolveImageURL(rawURL, upstreamBaseURL)
	if err != nil {
		return "", err
	}

	data, contentType, err := downloadFileBytes(resolvedURL, fallbackContentType)
	if err != nil {
		return "", err
	}

	objectKey := buildAliyunOssObjectKey(cfg.PathPrefix, contentType)
	if err := uploadBytesToAliyunOSS(cfg, objectKey, data, contentType); err != nil {
		return "", err
	}

	return buildAliyunOssPublicURL(cfg, objectKey)
}

func SaveBase64ImageToAliyunOSS(data string, contentType string) (string, error) {
	cfg := GetAliyunOssConfig()
	if !cfg.IsEnabledAndValid() {
		return "", nil
	}

	data = strings.TrimSpace(data)
	if data == "" {
		return "", fmt.Errorf("image data is empty")
	}

	contentType = canonicalContentType(contentType)
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("inline data is not image, content-type=%s", contentType)
	}

	imageBytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	if len(imageBytes) == 0 {
		return "", fmt.Errorf("decoded image is empty")
	}
	maxFileSize := int64(constant.MaxFileDownloadMB*1024*1024) + 1
	if int64(len(imageBytes)) >= maxFileSize {
		return "", fmt.Errorf("image size exceeds maximum allowed size")
	}

	detectedContentType := http.DetectContentType(imageBytes)
	if detectedContentType != "application/octet-stream" && strings.HasPrefix(detectedContentType, "image/") {
		contentType = detectedContentType
	}

	objectKey := buildAliyunOssObjectKey(cfg.PathPrefix, contentType)
	if err := uploadBytesToAliyunOSS(cfg, objectKey, imageBytes, contentType); err != nil {
		return "", err
	}

	return buildAliyunOssPublicURL(cfg, objectKey)
}

func resolveImageURL(rawURL string, upstreamBaseURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("image url is empty")
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	if parsedURL.IsAbs() {
		return parsedURL.String(), nil
	}

	if upstreamBaseURL == "" {
		return "", fmt.Errorf("relative image url %q requires upstream base url", rawURL)
	}

	baseURL, err := url.Parse(upstreamBaseURL)
	if err != nil {
		return "", err
	}
	return baseURL.ResolveReference(parsedURL).String(), nil
}

func downloadImageBytes(originURL string) ([]byte, string, error) {
	resp, err := DoDownloadRequest(originURL, "aliyun_oss_image_replace")
	if err != nil {
		return nil, "", err
	}
	defer CloseResponseBodyGracefully(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download image failed with status %d", resp.StatusCode)
	}

	maxFileSize := int64(constant.MaxFileDownloadMB*1024*1024) + 1
	limitedReader := io.LimitReader(resp.Body, maxFileSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) >= maxFileSize {
		return nil, "", fmt.Errorf("image size exceeds maximum allowed size")
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("downloaded image is empty")
	}

	contentType := canonicalContentType(resp.Header.Get("Content-Type"))
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("downloaded file is not image, content-type=%s", contentType)
	}

	return data, contentType, nil
}

func downloadFileBytes(originURL string, fallbackContentType string) ([]byte, string, error) {
	resp, err := DoDownloadRequest(originURL, "aliyun_oss_file_replace")
	if err != nil {
		return nil, "", err
	}
	defer CloseResponseBodyGracefully(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download file failed with status %d", resp.StatusCode)
	}

	maxFileSize := int64(constant.MaxFileDownloadMB*1024*1024) + 1
	limitedReader := io.LimitReader(resp.Body, maxFileSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) >= maxFileSize {
		return nil, "", fmt.Errorf("file size exceeds maximum allowed size")
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("downloaded file is empty")
	}

	contentType := canonicalContentType(resp.Header.Get("Content-Type"))
	if contentType == "" || contentType == "application/octet-stream" || contentType == "text/plain" {
		contentType = canonicalContentType(fallbackContentType)
	}
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

func canonicalContentType(contentType string) string {
	if contentType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return contentType
	}
	return mediaType
}

func buildAliyunOssObjectKey(prefix, contentType string) string {
	fileExt := imageFileExt(contentType)
	if prefix == "" {
		prefix = "openai-images"
	}
	prefix = strings.Trim(strings.ReplaceAll(prefix, "\\", "/"), "/")
	now := time.Now().UTC()
	fileName := uuid.NewString() + fileExt
	return path.Join(prefix, now.Format("2006/01/02"), fileName)
}

func imageFileExt(contentType string) string {
	exts, err := mime.ExtensionsByType(contentType)
	if err == nil {
		for _, ext := range exts {
			if ext != "" {
				return ext
			}
		}
	}
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/bmp":
		return ".bmp"
	case "image/svg+xml":
		return ".svg"
	case "application/postscript", "application/eps", "image/x-eps":
		return ".eps"
	default:
		return ".png"
	}
}

func uploadBytesToAliyunOSS(cfg AliyunOssConfig, objectKey string, data []byte, contentType string) error {
	uploadBaseURL, err := buildAliyunOssUploadBaseURL(cfg)
	if err != nil {
		return err
	}

	objectKey = strings.TrimLeft(objectKey, "/")
	putURL := strings.TrimRight(uploadBaseURL, "/") + "/" + objectKey
	date := time.Now().UTC().Format(http.TimeFormat)

	stringToSign := strings.Join([]string{
		http.MethodPut,
		"",
		contentType,
		date,
		fmt.Sprintf("/%s/%s", cfg.Bucket, objectKey),
	}, "\n")

	mac := hmac.New(sha1.New, []byte(cfg.AccessKeySecret))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(http.MethodPut, putURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Date", date)
	req.Header.Set("Authorization", fmt.Sprintf("OSS %s:%s", cfg.AccessKeyId, signature))
	req.ContentLength = int64(len(data))

	resp, err := GetHttpClient().Do(req)
	if err != nil {
		return err
	}
	defer CloseResponseBodyGracefully(resp)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("aliyun oss upload failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func buildAliyunOssUploadBaseURL(cfg AliyunOssConfig) (string, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("aliyun oss endpoint is empty")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}

	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	host := parsedURL.Host
	if host == "" {
		host = parsedURL.Path
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("aliyun oss endpoint host is empty")
	}

	scheme := parsedURL.Scheme
	if scheme == "" {
		scheme = "https"
	}

	if strings.HasPrefix(host, cfg.Bucket+".") {
		return fmt.Sprintf("%s://%s", scheme, host), nil
	}
	return fmt.Sprintf("%s://%s.%s", scheme, cfg.Bucket, host), nil
}

func buildAliyunOssPublicURL(cfg AliyunOssConfig, objectKey string) (string, error) {
	objectKey = strings.TrimLeft(objectKey, "/")
	if cfg.PublicBaseURL != "" {
		baseURL := strings.TrimRight(cfg.PublicBaseURL, "/")
		if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
			baseURL = "https://" + baseURL
		}
		return baseURL + "/" + objectKey, nil
	}

	uploadBaseURL, err := buildAliyunOssUploadBaseURL(cfg)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(uploadBaseURL, "/") + "/" + objectKey, nil
}
