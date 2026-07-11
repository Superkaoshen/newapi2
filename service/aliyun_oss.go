package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
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
	UploadTimeout   int
}

type R2Config struct {
	Enabled         bool
	Endpoint        string
	Bucket          string
	AccessKeyId     string
	AccessKeySecret string
	PathPrefix      string
	PublicBaseURL   string
	Region          string
	UploadTimeout   int
}

var markdownImageURLRegex = regexp.MustCompile(`!\[[^\]]*]\(([^)\r\n]+)\)`)

const (
	objectStorageProviderDisabled     = "disabled"
	objectStorageProviderAliyunOSS    = "aliyun_oss"
	objectStorageProviderCloudflareR2 = "cloudflare_r2"
	trustedImageDownloadTimeout       = 30 * time.Second
)

func getImageStorageProviderLocked() string {
	provider := strings.TrimSpace(common.OptionMap["ImageStorageProvider"])
	switch provider {
	case objectStorageProviderDisabled, objectStorageProviderAliyunOSS, objectStorageProviderCloudflareR2:
		return provider
	}
	if strings.EqualFold(strings.TrimSpace(common.OptionMap["AliyunOssEnabled"]), "true") {
		return objectStorageProviderAliyunOSS
	}
	return objectStorageProviderDisabled
}

func GetImageStorageProvider() string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return getImageStorageProviderLocked()
}

func GetAliyunOssConfig() AliyunOssConfig {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()

	return AliyunOssConfig{
		Enabled:         getImageStorageProviderLocked() == objectStorageProviderAliyunOSS,
		Endpoint:        strings.TrimSpace(common.OptionMap["AliyunOssEndpoint"]),
		Bucket:          strings.TrimSpace(common.OptionMap["AliyunOssBucket"]),
		AccessKeyId:     strings.TrimSpace(common.OptionMap["AliyunOssAccessKeyId"]),
		AccessKeySecret: strings.TrimSpace(common.OptionMap["AliyunOssAccessKeySecret"]),
		PathPrefix:      strings.TrimSpace(common.OptionMap["AliyunOssPathPrefix"]),
		PublicBaseURL:   strings.TrimSpace(common.OptionMap["AliyunOssPublicBaseUrl"]),
		UploadTimeout:   common.String2Int(strings.TrimSpace(common.OptionMap["AliyunOssUploadTimeoutSeconds"])),
	}
}

func GetR2Config() R2Config {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()

	return R2Config{
		Enabled:         getImageStorageProviderLocked() == objectStorageProviderCloudflareR2,
		Endpoint:        strings.TrimSpace(common.OptionMap["R2Endpoint"]),
		Bucket:          strings.TrimSpace(common.OptionMap["R2Bucket"]),
		AccessKeyId:     strings.TrimSpace(common.OptionMap["R2AccessKeyId"]),
		AccessKeySecret: strings.TrimSpace(common.OptionMap["R2AccessKeySecret"]),
		PathPrefix:      strings.TrimSpace(common.OptionMap["R2PathPrefix"]),
		PublicBaseURL:   strings.TrimSpace(common.OptionMap["R2PublicBaseUrl"]),
		Region:          strings.TrimSpace(common.OptionMap["R2Region"]),
		UploadTimeout:   common.String2Int(strings.TrimSpace(common.OptionMap["R2UploadTimeoutSeconds"])),
	}
}

func (c AliyunOssConfig) IsEnabledAndValid() bool {
	return c.Enabled &&
		c.Endpoint != "" &&
		c.Bucket != "" &&
		c.AccessKeyId != "" &&
		c.AccessKeySecret != ""
}

func (c R2Config) IsEnabledAndValid() bool {
	return c.Enabled &&
		c.Endpoint != "" &&
		c.Bucket != "" &&
		c.AccessKeyId != "" &&
		c.AccessKeySecret != ""
}

func IsAliyunOssEnabled() bool {
	return GetAliyunOssConfig().IsEnabledAndValid()
}

func IsObjectStorageEnabled() bool {
	return GetAliyunOssConfig().IsEnabledAndValid() || GetR2Config().IsEnabledAndValid()
}

func ReplaceMarkdownImageURLsWithAliyunOSS(content string, upstreamBaseURL string) (string, bool) {
	if !IsObjectStorageEnabled() {
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
			common.SysError(fmt.Sprintf("failed to save image url to object storage: %s", err.Error()))
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
	switch GetImageStorageProvider() {
	case objectStorageProviderAliyunOSS:
		cfg := GetAliyunOssConfig()
		if !cfg.IsEnabledAndValid() {
			return rawURL, nil
		}
		return saveImageURLToAliyunOSSWithConfig(cfg, rawURL, upstreamBaseURL)
	case objectStorageProviderCloudflareR2:
		cfg := GetR2Config()
		if !cfg.IsEnabledAndValid() {
			return rawURL, nil
		}
		return saveImageURLToR2WithConfig(cfg, rawURL, upstreamBaseURL)
	default:
		return rawURL, nil
	}
}

func StrictSaveImageURLToAliyunOSS(rawURL string, upstreamBaseURL string) (string, error) {
	return strictSaveURLToObjectStorage(rawURL, upstreamBaseURL, true)
}

func StrictSaveFileURLToAliyunOSS(rawURL string, upstreamBaseURL string) (string, error) {
	return strictSaveURLToObjectStorage(rawURL, upstreamBaseURL, false)
}

// StrictSaveTrustedImageURLToAliyunOSS permits private-network downloads only
// when the image URL is same-origin with the configured upstream base URL.
func StrictSaveTrustedImageURLToAliyunOSS(rawURL string, upstreamBaseURL string) (string, error) {
	return strictSaveTrustedURLToObjectStorage(rawURL, upstreamBaseURL, true)
}

func strictSaveURLToObjectStorage(rawURL string, upstreamBaseURL string, requireImage bool) (string, error) {
	switch GetImageStorageProvider() {
	case objectStorageProviderAliyunOSS:
		cfg := GetAliyunOssConfig()
		if !cfg.IsEnabledAndValid() {
			return "", fmt.Errorf("aliyun oss or cloudflare r2 is not enabled or configured")
		}
		return saveURLToAliyunOSSWithConfig(cfg, rawURL, upstreamBaseURL, requireImage)
	case objectStorageProviderCloudflareR2:
		cfg := GetR2Config()
		if !cfg.IsEnabledAndValid() {
			return "", fmt.Errorf("aliyun oss or cloudflare r2 is not enabled or configured")
		}
		return saveURLToR2WithConfig(cfg, rawURL, upstreamBaseURL, requireImage)
	default:
		return "", fmt.Errorf("aliyun oss or cloudflare r2 is not enabled or configured")
	}
}

func strictSaveTrustedURLToObjectStorage(rawURL string, upstreamBaseURL string, requireImage bool) (string, error) {
	provider := GetImageStorageProvider()
	var aliyunCfg AliyunOssConfig
	var r2Cfg R2Config
	switch provider {
	case objectStorageProviderAliyunOSS:
		aliyunCfg = GetAliyunOssConfig()
		if !aliyunCfg.IsEnabledAndValid() {
			return "", fmt.Errorf("aliyun oss or cloudflare r2 is not enabled or configured")
		}
	case objectStorageProviderCloudflareR2:
		r2Cfg = GetR2Config()
		if !r2Cfg.IsEnabledAndValid() {
			return "", fmt.Errorf("aliyun oss or cloudflare r2 is not enabled or configured")
		}
	default:
		return "", fmt.Errorf("aliyun oss or cloudflare r2 is not enabled or configured")
	}

	resolvedURL, err := resolveImageURL(rawURL, upstreamBaseURL)
	if err != nil {
		return "", err
	}
	if err := validateSameOriginURL(resolvedURL, upstreamBaseURL); err != nil {
		return "", err
	}
	var data []byte
	var contentType string
	if isExplicitPrivateUpstreamHost(upstreamBaseURL) {
		data, contentType, err = downloadTrustedURLBytes(resolvedURL, requireImage)
	} else {
		data, contentType, err = downloadURLBytes(resolvedURL, requireImage)
	}
	if err != nil {
		return "", err
	}

	switch provider {
	case objectStorageProviderAliyunOSS:
		objectKey := buildAliyunOssObjectKeyWithFallback(aliyunCfg.PathPrefix, contentType, extensionFromURL(resolvedURL))
		if err := uploadBytesToAliyunOSS(aliyunCfg, objectKey, data, contentType); err != nil {
			return "", err
		}
		return buildAliyunOssPublicURL(aliyunCfg, objectKey)
	case objectStorageProviderCloudflareR2:
		objectKey := buildAliyunOssObjectKeyWithFallback(r2Cfg.PathPrefix, contentType, extensionFromURL(resolvedURL))
		if err := uploadBytesToR2(r2Cfg, objectKey, data, contentType); err != nil {
			return "", err
		}
		return buildR2PublicURL(r2Cfg, objectKey)
	default:
		return "", fmt.Errorf("aliyun oss or cloudflare r2 is not enabled or configured")
	}
}

func isExplicitPrivateUpstreamHost(upstreamBaseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(upstreamBaseURL))
	if err != nil {
		return false
	}
	ip := net.ParseIP(parsed.Hostname())
	return ip != nil && !ip.IsUnspecified() && common.IsPrivateIP(ip)
}

func validateSameOriginURL(rawURL, upstreamBaseURL string) error {
	resultURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return err
	}
	baseURL, err := url.Parse(strings.TrimSpace(upstreamBaseURL))
	if err != nil {
		return err
	}
	if resultURL.Scheme == "" || resultURL.Host == "" || baseURL.Scheme == "" || baseURL.Host == "" {
		return fmt.Errorf("trusted result URL and upstream base URL must be absolute")
	}
	if (!strings.EqualFold(resultURL.Scheme, "http") && !strings.EqualFold(resultURL.Scheme, "https")) ||
		(!strings.EqualFold(baseURL.Scheme, "http") && !strings.EqualFold(baseURL.Scheme, "https")) {
		return fmt.Errorf("trusted result URL must use HTTP or HTTPS")
	}
	if resultURL.User != nil || baseURL.User != nil {
		return fmt.Errorf("trusted result URL must not contain user information")
	}
	if !strings.EqualFold(resultURL.Scheme, baseURL.Scheme) || !strings.EqualFold(resultURL.Host, baseURL.Host) {
		return fmt.Errorf("result URL origin does not match upstream base URL")
	}
	return nil
}

func saveImageURLToR2WithConfig(cfg R2Config, rawURL string, upstreamBaseURL string) (string, error) {
	return saveURLToR2WithConfig(cfg, rawURL, upstreamBaseURL, true)
}

func saveURLToR2WithConfig(cfg R2Config, rawURL string, upstreamBaseURL string, requireImage bool) (string, error) {
	resolvedURL, err := resolveImageURL(rawURL, upstreamBaseURL)
	if err != nil {
		return "", err
	}

	data, contentType, err := downloadURLBytes(resolvedURL, requireImage)
	if err != nil {
		return "", err
	}

	objectKey := buildAliyunOssObjectKeyWithFallback(cfg.PathPrefix, contentType, extensionFromURL(resolvedURL))
	if err := uploadBytesToR2(cfg, objectKey, data, contentType); err != nil {
		return "", err
	}

	return buildR2PublicURL(cfg, objectKey)
}

func saveImageURLToAliyunOSSWithConfig(cfg AliyunOssConfig, rawURL string, upstreamBaseURL string) (string, error) {
	return saveURLToAliyunOSSWithConfig(cfg, rawURL, upstreamBaseURL, true)
}

func saveURLToAliyunOSSWithConfig(cfg AliyunOssConfig, rawURL string, upstreamBaseURL string, requireImage bool) (string, error) {
	resolvedURL, err := resolveImageURL(rawURL, upstreamBaseURL)
	if err != nil {
		return "", err
	}

	data, contentType, err := downloadURLBytes(resolvedURL, requireImage)
	if err != nil {
		return "", err
	}

	objectKey := buildAliyunOssObjectKeyWithFallback(cfg.PathPrefix, contentType, extensionFromURL(resolvedURL))
	if err := uploadBytesToAliyunOSS(cfg, objectKey, data, contentType); err != nil {
		return "", err
	}

	return buildAliyunOssPublicURL(cfg, objectKey)
}

func SaveBase64ImageToAliyunOSS(data string, contentType string) (string, error) {
	provider := GetImageStorageProvider()
	var aliyunCfg AliyunOssConfig
	var r2Cfg R2Config
	switch provider {
	case objectStorageProviderAliyunOSS:
		aliyunCfg = GetAliyunOssConfig()
		if !aliyunCfg.IsEnabledAndValid() {
			return "", nil
		}
	case objectStorageProviderCloudflareR2:
		r2Cfg = GetR2Config()
		if !r2Cfg.IsEnabledAndValid() {
			return "", nil
		}
	default:
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
	maxFileSize := maxObjectStorageDownloadBytes() + 1
	if int64(len(imageBytes)) >= maxFileSize {
		return "", fmt.Errorf("image size exceeds maximum allowed size")
	}

	if detectedContentType := detectImageContentTypeFromBytes(imageBytes); detectedContentType != "" {
		contentType = detectedContentType
	}

	switch provider {
	case objectStorageProviderAliyunOSS:
		objectKey := buildAliyunOssObjectKey(aliyunCfg.PathPrefix, contentType)
		if err := uploadBytesToAliyunOSS(aliyunCfg, objectKey, imageBytes, contentType); err != nil {
			return "", err
		}
		return buildAliyunOssPublicURL(aliyunCfg, objectKey)
	case objectStorageProviderCloudflareR2:
		objectKey := buildAliyunOssObjectKey(r2Cfg.PathPrefix, contentType)
		if err := uploadBytesToR2(r2Cfg, objectKey, imageBytes, contentType); err != nil {
			return "", err
		}
		return buildR2PublicURL(r2Cfg, objectKey)
	default:
		return "", nil
	}
}

func maxObjectStorageDownloadBytes() int64 {
	maxMB := constant.MaxFileDownloadMB
	if maxMB <= 0 {
		maxMB = 64
	}
	return int64(maxMB) * 1024 * 1024
}

func detectImageContentTypeFromBytes(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if _, format, err := decodeImageConfig(data); err == nil {
		switch strings.ToLower(format) {
		case "jpeg", "jpg":
			return "image/jpeg"
		case "png":
			return "image/png"
		case "gif":
			return "image/gif"
		case "webp":
			return "image/webp"
		case "bmp":
			return "image/bmp"
		case "tiff":
			return "image/tiff"
		case "heic":
			return "image/heic"
		case "heif":
			return "image/heif"
		default:
			if strings.TrimSpace(format) != "" {
				return "image/" + strings.ToLower(format)
			}
		}
	}
	if heifMime := detectHEIF(data); heifMime != "" {
		return heifMime
	}
	detectedContentType := http.DetectContentType(data)
	if detectedContentType != "application/octet-stream" && strings.HasPrefix(detectedContentType, "image/") {
		return canonicalContentType(detectedContentType)
	}
	return ""
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
	return downloadURLBytes(originURL, true)
}

func downloadURLBytes(originURL string, requireImage bool) ([]byte, string, error) {
	resp, err := DoDownloadRequest(originURL, "object_storage_url_replace")
	if err != nil {
		return nil, "", err
	}
	defer CloseResponseBodyGracefully(resp)
	return readDownloadedURLResponse(resp, requireImage)
}

func downloadTrustedURLBytes(originURL string, requireImage bool) ([]byte, string, error) {
	client := GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	trustedClient := *client
	if transport, ok := trustedClient.Transport.(*http.Transport); ok && transport != nil {
		directTransport := transport.Clone()
		directTransport.Proxy = nil
		trustedClient.Transport = directTransport
	} else if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		directTransport := defaultTransport.Clone()
		directTransport.Proxy = nil
		trustedClient.Transport = directTransport
	}
	if trustedClient.Timeout <= 0 || trustedClient.Timeout > trustedImageDownloadTimeout {
		trustedClient.Timeout = trustedImageDownloadTimeout
	}
	trustedClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 {
			return nil
		}
		if err := validateSameOriginURL(req.URL.String(), via[0].URL.String()); err != nil {
			return err
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	resp, err := trustedClient.Get(originURL)
	if err != nil {
		return nil, "", err
	}
	defer CloseResponseBodyGracefully(resp)
	data, contentType, err := readDownloadedURLResponse(resp, requireImage)
	if err != nil || !requireImage {
		return data, contentType, err
	}
	detectedType := canonicalContentType(http.DetectContentType(data))
	if !isTrustedRasterContentType(detectedType) {
		return nil, "", fmt.Errorf("downloaded file is not a supported raster image, detected content-type=%s", detectedType)
	}
	return data, detectedType, nil
}

func isTrustedRasterContentType(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(canonicalContentType(contentType))) {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return true
	default:
		return false
	}
}

func readDownloadedURLResponse(resp *http.Response, requireImage bool) ([]byte, string, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download image failed with status %d", resp.StatusCode)
	}

	maxFileSize := maxObjectStorageDownloadBytes() + 1
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
	if requireImage && !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("downloaded file is not image, content-type=%s", contentType)
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
	return buildAliyunOssObjectKeyWithFallback(prefix, contentType, "")
}

func buildAliyunOssObjectKeyWithFallback(prefix, contentType string, fallbackExt string) string {
	fileExt := imageFileExt(contentType)
	if fallbackExt != "" && (fileExt == ".png" || fileExt == ".bin") {
		fileExt = fallbackExt
	}
	if prefix == "" {
		prefix = "openai-images"
	}
	prefix = strings.Trim(strings.ReplaceAll(prefix, "\\", "/"), "/")
	now := time.Now().UTC()
	fileName := uuid.NewString() + fileExt
	return path.Join(prefix, now.Format("2006/01/02"), fileName)
}

func imageFileExt(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(canonicalContentType(contentType)))
	if strings.Contains(contentType, "jpeg") || strings.Contains(contentType, "jpg") || strings.Contains(contentType, "jfif") {
		return ".jpg"
	}
	switch contentType {
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
	case "image/vnd.adobe.photoshop":
		return ".psd"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "application/octet-stream":
		return ".bin"
	}
	exts, err := mime.ExtensionsByType(contentType)
	if err == nil {
		for _, ext := range exts {
			if ext != "" && ext != ".jfif" {
				return ext
			}
		}
	}
	if !strings.HasPrefix(contentType, "image/") {
		return ".bin"
	}
	switch contentType {
	default:
		return ".png"
	}
}

func extensionFromURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(path.Ext(parsedURL.Path))
	if len(ext) < 2 || len(ext) > 10 {
		return ""
	}
	for _, r := range ext[1:] {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return ""
	}
	return ext
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

	timeout := cfg.UploadTimeout
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	common.SysLog(fmt.Sprintf("uploading to aliyun oss: endpoint=%s, bucket=%s, key=%s, size=%d, timeout=%ds", common.MaskSensitiveInfo(uploadBaseURL), cfg.Bucket, objectKey, len(data), timeout))

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(data))
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

func uploadBytesToR2(cfg R2Config, objectKey string, data []byte, contentType string) error {
	uploadBaseURL, err := buildR2UploadBaseURL(cfg)
	if err != nil {
		return err
	}

	objectKey = strings.TrimLeft(objectKey, "/")
	contentType = canonicalContentType(contentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	putURL := strings.TrimRight(uploadBaseURL, "/") + "/" + escapeR2Path(objectKey)
	parsedURL, err := url.Parse(putURL)
	if err != nil {
		return err
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("cloudflare r2 upload url host is empty")
	}

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	region := normalizeR2Region(cfg.Region)
	payloadHash := sha256Hex(data)

	headers := map[string]string{
		"content-type":         contentType,
		"host":                 parsedURL.Host,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
	}
	canonicalHeaders, signedHeaders := buildR2CanonicalHeaders(headers)
	canonicalRequest := strings.Join([]string{
		http.MethodPut,
		parsedURL.EscapedPath(),
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(r2HMACSHA256(deriveR2SigningKey(cfg.AccessKeySecret, dateStamp, region), []byte(stringToSign)))
	authorization := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		cfg.AccessKeyId,
		credentialScope,
		signedHeaders,
		signature,
	)

	timeout := cfg.UploadTimeout
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	common.SysLog(fmt.Sprintf("uploading to cloudflare r2: endpoint=%s, bucket=%s, key=%s, size=%d, timeout=%ds", common.MaskSensitiveInfo(uploadBaseURL), cfg.Bucket, objectKey, len(data), timeout))

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, parsedURL.String(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Host = parsedURL.Host
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", amzDate)
	req.ContentLength = int64(len(data))

	resp, err := GetHttpClient().Do(req)
	if err != nil {
		return err
	}
	defer CloseResponseBodyGracefully(resp)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("cloudflare r2 upload failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func buildR2UploadBaseURL(cfg R2Config) (string, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("cloudflare r2 endpoint is empty")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}

	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	host := strings.TrimSpace(parsedURL.Host)
	if host == "" {
		host = strings.TrimSpace(parsedURL.Path)
		parsedURL.Path = ""
	}
	if host == "" {
		return "", fmt.Errorf("cloudflare r2 endpoint host is empty")
	}

	bucket := strings.Trim(strings.TrimSpace(cfg.Bucket), "/")
	if bucket == "" {
		return "", fmt.Errorf("cloudflare r2 bucket is empty")
	}

	scheme := parsedURL.Scheme
	if scheme == "" {
		scheme = "https"
	}

	pathParts := make([]string, 0, 2)
	basePath := strings.Trim(parsedURL.EscapedPath(), "/")
	if basePath != "" && strings.Trim(parsedURL.Path, "/") != bucket {
		pathParts = append(pathParts, basePath)
	}
	pathParts = append(pathParts, url.PathEscape(bucket))

	return fmt.Sprintf("%s://%s/%s", scheme, host, strings.Join(pathParts, "/")), nil
}

func buildR2PublicURL(cfg R2Config, objectKey string) (string, error) {
	objectKey = strings.TrimLeft(objectKey, "/")
	if cfg.PublicBaseURL != "" {
		baseURL := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
		if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
			baseURL = "https://" + baseURL
		}
		return baseURL + "/" + escapeR2Path(objectKey), nil
	}

	uploadBaseURL, err := buildR2UploadBaseURL(cfg)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(uploadBaseURL, "/") + "/" + escapeR2Path(objectKey), nil
}

func normalizeR2Region(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return "auto"
	}
	return region
}

func buildR2CanonicalHeaders(headers map[string]string) (string, string) {
	names := make([]string, 0, len(headers))
	normalized := make(map[string]string, len(headers))
	for name, value := range headers {
		lowerName := strings.ToLower(strings.TrimSpace(name))
		if lowerName == "" {
			continue
		}
		names = append(names, lowerName)
		normalized[lowerName] = strings.Join(strings.Fields(value), " ")
	}
	sort.Strings(names)

	var builder strings.Builder
	for _, name := range names {
		builder.WriteString(name)
		builder.WriteByte(':')
		builder.WriteString(normalized[name])
		builder.WriteByte('\n')
	}
	return builder.String(), strings.Join(names, ";")
}

func deriveR2SigningKey(secret string, dateStamp string, region string) []byte {
	dateKey := r2HMACSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	regionKey := r2HMACSHA256(dateKey, []byte(region))
	serviceKey := r2HMACSHA256(regionKey, []byte("s3"))
	return r2HMACSHA256(serviceKey, []byte("aws4_request"))
}

func r2HMACSHA256(key []byte, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func escapeR2Path(rawPath string) string {
	rawPath = strings.TrimLeft(rawPath, "/")
	if rawPath == "" {
		return ""
	}
	segments := strings.Split(rawPath, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
