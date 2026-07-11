package mihuifang

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type fireflyChatRequest struct {
	Model    string               `json:"model"`
	Messages []fireflyChatMessage `json:"messages"`
	N        *int                 `json:"n,omitempty"`
}

type fireflyChatMessage struct {
	Role    string                   `json:"role"`
	Content []fireflyChatContentPart `json:"content"`
}

type fireflyChatContentPart struct {
	Type     string               `json:"type"`
	Text     string               `json:"text,omitempty"`
	ImageURL *fireflyChatImageURL `json:"image_url,omitempty"`
}

type fireflyChatImageURL struct {
	URL string `json:"url"`
}

type fireflyChatResponse struct {
	ID      string              `json:"id,omitempty"`
	Choices []fireflyChatChoice `json:"choices,omitempty"`
	Error   *fireflyChatError   `json:"error,omitempty"`
}

type fireflyChatChoice struct {
	Message fireflyChatResponseMessage `json:"message"`
}

type fireflyChatResponseMessage struct {
	Content json.RawMessage `json:"content"`
}

type fireflyChatError struct {
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}

type fireflyResponseContentPart struct {
	Type     string               `json:"type,omitempty"`
	URL      string               `json:"url,omitempty"`
	ImageURL *fireflyChatImageURL `json:"image_url,omitempty"`
}

var (
	fireflyCompleteModelPattern = regexp.MustCompile(`(?i)^(firefly-(?:nano-banana|nano-banana2|nano-banana-pro|gpt-image))-(1k|2k|4k)-(\d{1,2})x(\d{1,2})$`)
	fireflyModelTierPattern     = regexp.MustCompile(`(?i)-(1k|2k|4k)-\d{1,2}x\d{1,2}$`)
	fireflyPixelOptionPattern   = regexp.MustCompile(`(?i)^\s*(\d{3,5})\s*x\s*(\d{3,5})(?:[-_\s]+(1k|2k|4k))?\s*$`)
	fireflyInternalModelPattern = regexp.MustCompile(`(?i)firef?ly-(?:nano-banana(?:2|-pro)?|gpt-image)(?:-[a-z0-9_.:@]+)*`)
	fireflyFamilyModelPattern   = regexp.MustCompile(`(?i)\b(?:nanobanana(?:2|pro)?|nano-banana(?:2|-pro)?|gpt-image-2)\b`)
	markdownImagePattern        = regexp.MustCompile(`!\[[^\]]*]\(\s*<?([^\s)>]+)>?(?:\s+["'][^)]*["'])?\s*\)`)
)

type fireflyModelSpec struct {
	Model  string
	Family string
	Tier   string
	Aspect string
}

type fireflyRequestedOptions struct {
	Tier       string
	Aspect     string
	Quality    string
	HasTier    bool
	HasAspect  bool
	HasQuality bool
}

func fireflyModelTier(modelName string) string {
	match := fireflyModelTierPattern.FindStringSubmatch(strings.TrimSpace(modelName))
	if len(match) != 2 {
		return ""
	}
	return strings.ToLower(match[1])
}

func buildFireflyRequestBody(c *gin.Context, upstreamModel string, req relaycommon.TaskSubmitReq) (io.Reader, error) {
	if req.OutputPSD != nil && *req.OutputPSD {
		return nil, fmt.Errorf("output_psd is not supported by this image channel")
	}
	modelName, err := buildFireflyModelName(upstreamModel, req)
	if err != nil {
		return nil, err
	}
	images, err := fireflyRequestImages(c, upstreamModel, req)
	if err != nil {
		return nil, err
	}

	content := make([]fireflyChatContentPart, 0, len(images)+1)
	content = append(content, fireflyChatContentPart{Type: "text", Text: req.Prompt})
	for _, imageURL := range images {
		content = append(content, fireflyChatContentPart{
			Type:     "image_url",
			ImageURL: &fireflyChatImageURL{URL: imageURL},
		})
	}

	body := fireflyChatRequest{
		Model: modelName,
		Messages: []fireflyChatMessage{{
			Role:    "user",
			Content: content,
		}},
	}
	if req.N > 1 {
		body.N = common.GetPointer(req.N)
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func buildFireflyModelName(upstreamModel string, req relaycommon.TaskSubmitReq) (string, error) {
	spec, err := resolveFireflyModelSpec(upstreamModel, req)
	if err != nil {
		return "", err
	}
	return spec.Model, nil
}

func resolveFireflyModelSpec(upstreamModel string, req relaycommon.TaskSubmitReq) (fireflyModelSpec, error) {
	upstreamModel = strings.ToLower(strings.TrimSpace(upstreamModel))
	if match := fireflyCompleteModelPattern.FindStringSubmatch(upstreamModel); len(match) == 5 {
		family := mihuifangModelFamily(match[1])
		spec := fireflyModelSpec{
			Model:  upstreamModel,
			Family: family,
			Tier:   strings.ToLower(match[2]),
			Aspect: match[3] + ":" + match[4],
		}
		if err := validateFireflyAspect(family, spec.Aspect); err != nil {
			return fireflyModelSpec{}, err
		}
		requested, err := parseFireflyRequestedOptions(family, req)
		if err != nil {
			return fireflyModelSpec{}, err
		}
		if requested.HasTier && requested.Tier != spec.Tier {
			return fireflyModelSpec{}, fmt.Errorf("requested image resolution conflicts with the configured model")
		}
		if requested.HasAspect && requested.Aspect != spec.Aspect {
			return fireflyModelSpec{}, fmt.Errorf("requested image aspect ratio conflicts with the configured model")
		}
		if err := validateFireflyQualityForTier(family, requested, spec.Tier); err != nil {
			return fireflyModelSpec{}, err
		}
		return spec, nil
	}

	var prefix string
	family := mihuifangModelFamily(upstreamModel)
	switch family {
	case "nanobanana":
		prefix = "firefly-nano-banana"
	case "nanobanana2":
		prefix = "firefly-nano-banana2"
	case "nanobananapro":
		prefix = "firefly-nano-banana-pro"
	case "gpt-image-2":
		prefix = "firefly-gpt-image"
	default:
		return fireflyModelSpec{}, fmt.Errorf("unsupported image model mapping")
	}

	requested, err := parseFireflyRequestedOptions(family, req)
	if err != nil {
		return fireflyModelSpec{}, err
	}
	tier := requested.Tier
	if !requested.HasTier {
		tier = defaultFireflyTier(family, requested)
	}
	if err := validateFireflyQualityForTier(family, requested, tier); err != nil {
		return fireflyModelSpec{}, err
	}
	aspect := requested.Aspect
	if !requested.HasAspect {
		aspect = "1:1"
	}
	if err := validateFireflyAspect(family, aspect); err != nil {
		return fireflyModelSpec{}, err
	}
	return fireflyModelSpec{
		Model:  fmt.Sprintf("%s-%s-%s", prefix, tier, strings.ReplaceAll(aspect, ":", "x")),
		Family: family,
		Tier:   tier,
		Aspect: aspect,
	}, nil
}

func parseFireflyRequestedOptions(family string, req relaycommon.TaskSubmitReq) (fireflyRequestedOptions, error) {
	var options fireflyRequestedOptions
	quality := strings.ToLower(strings.TrimSpace(req.Quality))
	if quality != "" {
		options.Quality = quality
		options.HasQuality = true
		switch family {
		case "gpt-image-2":
			switch quality {
			case "low":
				options.Tier, options.HasTier = "1k", true
			case "medium":
				options.Tier, options.HasTier = "2k", true
			case "high":
				options.Tier, options.HasTier = "4k", true
			default:
				return fireflyRequestedOptions{}, fmt.Errorf("unsupported quality for gpt-image-2: %s", req.Quality)
			}
		default:
			if quality != "standard" && quality != "hd" {
				return fireflyRequestedOptions{}, fmt.Errorf("unsupported quality for %s: %s", family, req.Quality)
			}
		}
	}

	if err := mergeFireflyTextOptions(&options, family, "size", req.Size); err != nil {
		return fireflyRequestedOptions{}, err
	}
	if err := mergeFireflyTextOptions(&options, family, "resolution", req.Resolution); err != nil {
		return fireflyRequestedOptions{}, err
	}
	rawAspect := strings.TrimSpace(req.AspectRatio)
	aspect := imageAspectFromText(rawAspect)
	if rawAspect != "" && !strings.EqualFold(rawAspect, "auto") && aspect == "" {
		return fireflyRequestedOptions{}, fmt.Errorf("unsupported aspect_ratio: %s", req.AspectRatio)
	}
	if err := mergeFireflyAspect(&options, "aspect_ratio", aspect); err != nil {
		return fireflyRequestedOptions{}, err
	}
	return options, nil
}

func mergeFireflyTextOptions(options *fireflyRequestedOptions, family, field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "auto") {
		return nil
	}
	if match := fireflyPixelOptionPattern.FindStringSubmatch(value); len(match) == 4 {
		width, errWidth := strconv.Atoi(match[1])
		height, errHeight := strconv.Atoi(match[2])
		if errWidth != nil || errHeight != nil || width <= 0 || height <= 0 {
			return fmt.Errorf("invalid image size: %s", value)
		}
		pixelSize := fmt.Sprintf("%dx%d", width, height)
		if spec, exists := aiAPIProSizeByPixels[pixelSize]; exists {
			if family != "gpt-image-2" {
				if err := mergeFireflyTier(options, field, spec.Tier); err != nil {
					return err
				}
			}
			if match[3] != "" {
				if err := mergeFireflyTier(options, field, strings.ToLower(match[3])); err != nil {
					return err
				}
			}
			return mergeFireflyAspect(options, field, spec.Aspect)
		}
		if family != "gpt-image-2" {
			return fmt.Errorf("unsupported size for %s: %s", family, value)
		}
		divisor := greatestCommonDivisor(width, height)
		if divisor <= 0 {
			return fmt.Errorf("invalid image size: %s", value)
		}
		if match[3] != "" {
			if err := mergeFireflyTier(options, field, strings.ToLower(match[3])); err != nil {
				return err
			}
		}
		return mergeFireflyAspect(options, field, strconv.Itoa(width/divisor)+":"+strconv.Itoa(height/divisor))
	}
	if _, _, ok := parsePixels(value); ok {
		return fmt.Errorf("unsupported %s: %s", field, value)
	}

	tier := imageTierFromText(value)
	if tier != "" {
		if err := mergeFireflyTier(options, field, tier); err != nil {
			return err
		}
	}
	aspect := imageAspectFromText(value)
	if aspect == "auto" {
		aspect = ""
	}
	if aspect != "" {
		if err := mergeFireflyAspect(options, field, aspect); err != nil {
			return err
		}
	}
	if tier == "" && aspect == "" {
		return fmt.Errorf("unsupported %s: %s", field, value)
	}
	return nil
}

func mergeFireflyTier(options *fireflyRequestedOptions, field, tier string) error {
	if options.HasTier && options.Tier != tier {
		return fmt.Errorf("conflicting image resolution in %s", field)
	}
	options.Tier = tier
	options.HasTier = true
	return nil
}

func mergeFireflyAspect(options *fireflyRequestedOptions, field, aspect string) error {
	if aspect == "" || aspect == "auto" {
		return nil
	}
	if options.HasAspect && options.Aspect != aspect {
		return fmt.Errorf("conflicting image aspect ratio in %s", field)
	}
	options.Aspect = aspect
	options.HasAspect = true
	return nil
}

func defaultFireflyTier(family string, options fireflyRequestedOptions) string {
	if family != "gpt-image-2" && options.Quality == "hd" {
		return "2k"
	}
	return "1k"
}

func validateFireflyQualityForTier(family string, options fireflyRequestedOptions, tier string) error {
	if !options.HasQuality {
		return nil
	}
	if family == "gpt-image-2" {
		qualityTier := map[string]string{"low": "1k", "medium": "2k", "high": "4k"}[options.Quality]
		if qualityTier != tier {
			return fmt.Errorf("quality conflicts with the requested image resolution")
		}
		return nil
	}
	if options.Quality == "standard" && tier != "1k" {
		return fmt.Errorf("quality standard conflicts with the requested image resolution")
	}
	if options.Quality == "hd" && tier == "1k" {
		return fmt.Errorf("quality hd conflicts with the requested image resolution")
	}
	return nil
}

func validateFireflyAspect(family, aspect string) error {
	if family == "gpt-image-2" {
		parts := strings.Split(aspect, ":")
		if len(parts) != 2 {
			return fmt.Errorf("unsupported image aspect ratio: %s", aspect)
		}
		width, errWidth := strconv.Atoi(parts[0])
		height, errHeight := strconv.Atoi(parts[1])
		if errWidth != nil || errHeight != nil || width <= 0 || height <= 0 || width > 99 || height > 99 {
			return fmt.Errorf("unsupported image aspect ratio: %s", aspect)
		}
		return nil
	}
	specs, ok := aiAPIProSizeByAspectTier[aspect]
	if !ok {
		return fmt.Errorf("unsupported ratio for %s: %s", family, aspect)
	}
	for _, spec := range specs {
		if isImageSizeSupportedByModel(family, spec) {
			return nil
		}
	}
	return fmt.Errorf("%s does not support ratio %s", family, aspect)
}

func greatestCommonDivisor(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func fireflyRequestImages(c *gin.Context, upstreamModel string, req relaycommon.TaskSubmitReq) ([]string, error) {
	images := requestImages(req)
	images = append(images, req.ReferenceImages...)
	images = append(images, req.ReferenceImageURLs...)
	maskImages, err := rawImageInputs(req.Mask)
	if err != nil {
		return nil, err
	}
	images = append(images, maskImages...)
	images = uniqueNonEmptyStrings(images)

	if c != nil && c.Request != nil && strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return nil, fmt.Errorf("parse multipart form failed: %w", err)
		}
		for fieldName, fileHeaders := range form.File {
			switch normalizeMultipartFileField(fieldName) {
			case "image", "reference_images", "mask":
			default:
				continue
			}
			for _, fileHeader := range fileHeaders {
				dataURI, err := multipartImageDataURI(fileHeader)
				if err != nil {
					return nil, err
				}
				images = append(images, dataURI)
			}
		}
		images = append(images, form.Value["image"]...)
		images = append(images, form.Value["images"]...)
		images = append(images, form.Value["referenceImages"]...)
		images = append(images, form.Value["reference_images"]...)
		images = append(images, form.Value["reference_image_urls"]...)
		images = append(images, form.Value["mask"]...)
	}
	images = uniqueNonEmptyStrings(images)
	if err := validateImageInputLimit(upstreamModel, len(images)); err != nil {
		return nil, err
	}
	return normalizeFireflyImageInputs(images)
}

func normalizeFireflyImageInputs(inputs []string) ([]string, error) {
	normalized := make([]string, 0, len(inputs))
	for _, input := range inputs {
		value, err := normalizeFireflyImageInput(input)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func normalizeFireflyImageInput(input string) (string, error) {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(strings.ToLower(input), "data:image/") {
		return input, nil
	}
	if parsed, err := url.Parse(input); err == nil && parsed.Host != "" &&
		(strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
		return input, nil
	}

	data, err := base64.StdEncoding.DecodeString(input)
	if err != nil || len(data) == 0 {
		return "", fmt.Errorf("image input must be an HTTP URL, image data URI, or base64 image")
	}
	maxBytes := fireflyMaxImageBytes()
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("image size exceeds maximum allowed size")
	}
	contentType := canonicalImageContentType(http.DetectContentType(data))
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("base64 image input is not an image")
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func canonicalImageContentType(contentType string) string {
	if separator := strings.IndexByte(contentType, ';'); separator >= 0 {
		contentType = contentType[:separator]
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}

func fireflyMaxImageBytes() int64 {
	maxMB := constant.MaxFileDownloadMB
	if maxMB <= 0 {
		maxMB = 64
	}
	return int64(maxMB) * 1024 * 1024
}

func rawImageInputs(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value string
	if err := common.Unmarshal(raw, &value); err == nil {
		return []string{value}, nil
	}
	var values []string
	if err := common.Unmarshal(raw, &values); err == nil {
		return values, nil
	}
	return nil, fmt.Errorf("mask must be an image URL, data URI, or an array of them")
}

func multipartImageDataURI(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("open multipart file %q failed: %w", fileHeader.Filename, err)
	}
	defer file.Close()

	maxBytes := fireflyMaxImageBytes() + 1
	data, err := io.ReadAll(io.LimitReader(file, maxBytes))
	if err != nil {
		return "", fmt.Errorf("read multipart file %q failed: %w", fileHeader.Filename, err)
	}
	if int64(len(data)) >= maxBytes {
		return "", fmt.Errorf("image size exceeds maximum allowed size")
	}
	contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return "", fmt.Errorf("multipart file %q is not an image", fileHeader.Filename)
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (a *TaskAdaptor) doFireflyResponse(c *gin.Context, responseBody []byte, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	var upstream fireflyChatResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("invalid upstream image response"), "unmarshal_response_body_failed", http.StatusBadGateway)
	}
	if upstream.Error != nil {
		message := sanitizeFireflyText(firstNonEmpty(upstream.Error.Message, "upstream image generation failed"), info.OriginModelName)
		return "", nil, service.TaskErrorWrapper(errors.New(message), "upstream_task_failed", http.StatusBadGateway)
	}

	resultURLs := fireflyResponseImageURLs(upstream)
	if len(resultURLs) == 0 {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("upstream image response contains no image"), "invalid_response", http.StatusBadGateway)
	}
	if req, err := relaycommon.GetTaskRequest(c); err == nil {
		if err := validateFireflyOutputCount(req.N, len(resultURLs)); err != nil {
			return "", nil, service.TaskErrorWrapper(err, "invalid_response", http.StatusBadGateway)
		}
	}
	savedURLs := make([]string, 0, len(resultURLs))
	for _, resultURL := range resultURLs {
		savedURL, err := service.StrictSaveTrustedImageURLToAliyunOSS(resultURL, a.baseURL)
		if err != nil {
			common.SysError("save Firefly result image failed: " + err.Error())
			return "", nil, service.TaskErrorWrapperLocal(fmt.Errorf("save result image failed"), "save_result_file_failed", http.StatusInternalServerError)
		}
		savedURLs = append(savedURLs, savedURL)
	}
	savedURLs = uniqueNonEmptyStrings(savedURLs)
	if len(savedURLs) == 0 {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("object storage returned no image URL"), "save_result_file_failed", http.StatusInternalServerError)
	}

	publicResponse := fireflyPublicTaskResponse(info, savedURLs)
	taskData := normalizeResponseData(publicResponse)
	a.initialTaskInfo = &relaycommon.TaskInfo{
		TaskID:   info.PublicTaskID,
		Status:   model.TaskStatusSuccess,
		Progress: taskcommon.ProgressComplete,
		Url:      savedURLs[0],
		Data:     taskData,
	}
	c.JSON(http.StatusOK, publicResponse)
	return firstNonEmpty(upstream.ID, info.PublicTaskID), taskData, nil
}

func fireflyPublicTaskResponse(info *relaycommon.RelayInfo, savedURLs []string) aiAPIProTaskResponse {
	items := make([]interface{}, 0, len(savedURLs))
	for _, savedURL := range savedURLs {
		items = append(items, map[string]interface{}{
			"url":  savedURL,
			"type": "image",
		})
	}
	result := map[string]interface{}{
		"items":     items,
		"image_url": savedURLs[0],
		"url":       savedURLs[0],
	}
	if len(savedURLs) > 1 {
		result["image_urls"] = savedURLs
	}
	return aiAPIProTaskResponse{
		RequestID:     info.PublicTaskID,
		ModelCode:     info.OriginModelName,
		Status:        "succeeded",
		BillingStatus: "billed",
		Progress:      100,
		ResultCount:   len(savedURLs),
		Result:        result,
		URL:           savedURLs[0],
	}
}

func fireflyResponseImageURLs(response fireflyChatResponse) []string {
	urls := make([]string, 0)
	for _, choice := range response.Choices {
		var content string
		if err := common.Unmarshal(choice.Message.Content, &content); err == nil {
			urls = append(urls, extractMarkdownImageURLs(content)...)
			if candidate := strings.TrimSpace(content); strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
				urls = append(urls, candidate)
			}
			continue
		}
		var parts []fireflyResponseContentPart
		if err := common.Unmarshal(choice.Message.Content, &parts); err != nil {
			continue
		}
		for _, part := range parts {
			if part.ImageURL != nil {
				urls = append(urls, part.ImageURL.URL)
			}
			if strings.EqualFold(part.Type, "image_url") {
				urls = append(urls, part.URL)
			}
		}
	}
	return uniqueNonEmptyStrings(urls)
}

func extractMarkdownImageURLs(content string) []string {
	matches := markdownImagePattern.FindAllStringSubmatch(content, -1)
	urls := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			urls = append(urls, match[1])
		}
	}
	return uniqueNonEmptyStrings(urls)
}

func validateFireflyOutputCount(requested, actual int) error {
	expected := requested
	if expected <= 1 {
		expected = 1
	}
	if actual != expected {
		return fmt.Errorf("upstream returned %d images, expected %d", actual, expected)
	}
	return nil
}

func sanitizeFireflyErrorResponse(resp *http.Response, publicModel string) *http.Response {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		body = []byte("upstream image request failed")
	}
	_ = resp.Body.Close()
	sanitized := sanitizeFireflyText(string(body), publicModel)
	resp.Body = io.NopCloser(strings.NewReader(sanitized))
	resp.ContentLength = int64(len(sanitized))
	return resp
}

func sanitizeFireflyText(text, publicModel string) string {
	replacement := strings.TrimSpace(publicModel)
	if replacement == "" {
		replacement = "image model"
	}
	replaceUnlessPublic := func(value string) string {
		if strings.EqualFold(value, replacement) {
			return value
		}
		return replacement
	}
	text = fireflyInternalModelPattern.ReplaceAllStringFunc(text, replaceUnlessPublic)
	return fireflyFamilyModelPattern.ReplaceAllStringFunc(text, func(value string) string {
		if strings.Contains(strings.ToLower(replacement), strings.ToLower(value)) {
			return value
		}
		return replaceUnlessPublic(value)
	})
}

// SanitizePublicModelText removes internal image model aliases from text that
// can be returned through user-facing task records.
func SanitizePublicModelText(text, publicModel string) string {
	return sanitizeFireflyText(text, publicModel)
}

// SanitizePublicModelJSON applies model sanitization only to JSON string
// values, preserving valid JSON even when a public model contains escapes.
func SanitizePublicModelJSON(data []byte, publicModel string) []byte {
	if len(data) == 0 {
		return data
	}
	var value interface{}
	if err := common.Unmarshal(data, &value); err != nil {
		return data
	}
	value = sanitizePublicModelJSONValue(value, publicModel)
	sanitized, err := common.Marshal(value)
	if err != nil {
		return data
	}
	return sanitized
}

func sanitizePublicModelJSONValue(value interface{}, publicModel string) interface{} {
	switch typed := value.(type) {
	case string:
		return sanitizeFireflyText(typed, publicModel)
	case []interface{}:
		for i := range typed {
			typed[i] = sanitizePublicModelJSONValue(typed[i], publicModel)
		}
		return typed
	case map[string]interface{}:
		for key, item := range typed {
			typed[key] = sanitizePublicModelJSONValue(item, publicModel)
		}
		return typed
	default:
		return value
	}
}

func isHTTPStatusSuccess(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}
