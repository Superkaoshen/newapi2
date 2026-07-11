package mihuifang

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func TestBuildFireflyModelNameMapsMihuifangOptions(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		req      relaycommon.TaskSubmitReq
		expected string
	}{
		{
			name:     "nano banana default",
			model:    "nanobanana",
			req:      relaycommon.TaskSubmitReq{},
			expected: "firefly-nano-banana-1k-1x1",
		},
		{
			name:     "nano pro 2k landscape",
			model:    "nanobananapro",
			req:      relaycommon.TaskSubmitReq{Size: "16x9-2K"},
			expected: "firefly-nano-banana-pro-2k-16x9",
		},
		{
			name:     "nano banana2 pixel size",
			model:    "nanobanana2",
			req:      relaycommon.TaskSubmitReq{Size: "1536x2752"},
			expected: "firefly-nano-banana2-2k-9x16",
		},
		{
			name:     "gpt image quality",
			model:    "gpt-image-2",
			req:      relaycommon.TaskSubmitReq{Quality: "high", AspectRatio: "4:3"},
			expected: "firefly-gpt-image-4k-4x3",
		},
		{
			name:     "complete internal model with matching options",
			model:    "firefly-nano-banana-pro-2k-16x9",
			req:      relaycommon.TaskSubmitReq{Size: "16x9-2K", Quality: "hd"},
			expected: "firefly-nano-banana-pro-2k-16x9",
		},
		{
			name:     "nano quality fallback",
			model:    "nanobanana",
			req:      relaycommon.TaskSubmitReq{Quality: "hd", AspectRatio: "4:3"},
			expected: "firefly-nano-banana-2k-4x3",
		},
		{
			name:     "gpt custom pixels keep quality tier",
			model:    "gpt-image-2",
			req:      relaycommon.TaskSubmitReq{Size: "1536x1024", Quality: "high"},
			expected: "firefly-gpt-image-4k-3x2",
		},
		{
			name:     "gpt resolution without quality",
			model:    "gpt-image-2",
			req:      relaycommon.TaskSubmitReq{Resolution: "4K", AspectRatio: "16:9"},
			expected: "firefly-gpt-image-4k-16x9",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := buildFireflyModelName(test.model, test.req)
			if err != nil {
				t.Fatalf("buildFireflyModelName error = %v", err)
			}
			if got != test.expected {
				t.Fatalf("model = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestBuildFireflyModelNameRejectsConflictingOrInvalidOptions(t *testing.T) {
	tests := []struct {
		name  string
		model string
		req   relaycommon.TaskSubmitReq
	}{
		{
			name:  "complete model tier conflict",
			model: "firefly-nano-banana-pro-2k-16x9",
			req:   relaycommon.TaskSubmitReq{Size: "1x1-4K"},
		},
		{
			name:  "size and aspect conflict",
			model: "nanobanana2",
			req:   relaycommon.TaskSubmitReq{Size: "1536x2752", AspectRatio: "1:1"},
		},
		{
			name:  "gpt quality and resolution conflict",
			model: "gpt-image-2",
			req:   relaycommon.TaskSubmitReq{Quality: "low", Resolution: "4K"},
		},
		{
			name:  "invalid aspect ratio",
			model: "nanobananapro",
			req:   relaycommon.TaskSubmitReq{AspectRatio: "not-a-ratio"},
		},
		{
			name:  "invalid complete model aspect",
			model: "firefly-nano-banana-pro-2k-99x99",
		},
		{
			name:  "pixel suffix conflicts with actual pixel tier",
			model: "nanobanana",
			req:   relaycommon.TaskSubmitReq{Size: "2752x1536-4K"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := buildFireflyModelName(test.model, test.req); err == nil {
				t.Fatalf("buildFireflyModelName(%q, %#v) error = nil", test.model, test.req)
			}
		})
	}
}

func TestFireflyResolvedTierAndBillingKeyStayAligned(t *testing.T) {
	tests := []struct {
		model    string
		req      relaycommon.TaskSubmitReq
		wantTier string
		wantKey  string
	}{
		{model: "nanobanana", req: relaycommon.TaskSubmitReq{}, wantTier: "1k", wantKey: "@1k"},
		{model: "nanobanana", req: relaycommon.TaskSubmitReq{Quality: "hd"}, wantTier: "2k", wantKey: "@2k"},
		{model: "nanobanana2", req: relaycommon.TaskSubmitReq{Resolution: "4k"}, wantTier: "4k", wantKey: "@4k"},
		{model: "nanobananapro", req: relaycommon.TaskSubmitReq{Size: "16x9-2k"}, wantTier: "2k", wantKey: "@2k"},
		{model: "gpt-image-2", req: relaycommon.TaskSubmitReq{Quality: "low"}, wantTier: "1k", wantKey: "@low"},
		{model: "gpt-image-2", req: relaycommon.TaskSubmitReq{Quality: "medium"}, wantTier: "2k", wantKey: "@medium"},
		{model: "gpt-image-2", req: relaycommon.TaskSubmitReq{Quality: "high"}, wantTier: "4k", wantKey: "@high"},
		{model: "firefly-nano-banana-pro-4k-1x1", req: relaycommon.TaskSubmitReq{}, wantTier: "4k", wantKey: "@4k"},
	}

	for _, test := range tests {
		spec, err := resolveFireflyModelSpec(test.model, test.req)
		if err != nil {
			t.Fatalf("resolveFireflyModelSpec(%q) error = %v", test.model, err)
		}
		if spec.Tier != test.wantTier || fireflyTierPriceKey(spec) != test.wantKey {
			t.Fatalf("model=%q resolved tier/key = %q/%q, want %q/%q", test.model, spec.Tier, fireflyTierPriceKey(spec), test.wantTier, test.wantKey)
		}
	}
}

func TestFireflyRequestUsesChatCompletionsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType:    constant.ChannelTypeFirefly,
		ChannelBaseUrl: "http://127.0.0.1:6001/",
		ApiKey:         "service-key",
	}}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestURL, err := adaptor.BuildRequestURL(info)
	if err != nil {
		t.Fatalf("BuildRequestURL error = %v", err)
	}
	if requestURL != "http://127.0.0.1:6001/v1/chat/completions" {
		t.Fatalf("request URL = %q", requestURL)
	}
	request := httptest.NewRequest(http.MethodPost, requestURL, nil)
	if err := adaptor.BuildRequestHeader(c, request, info); err != nil {
		t.Fatalf("BuildRequestHeader error = %v", err)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer service-key" {
		t.Fatalf("authorization = %q", got)
	}
	if got := request.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
}

func TestFireflyValidationAllowsPublicModelBeforeChannelMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gemini-3-pro-image","prompt":"draw a cat","size":"16x9-2K"}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "gemini-3-pro-image",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeFirefly,
			UpstreamModelName: "gemini-3-pro-image",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("pre-mapping validation rejected public model: %#v", taskErr)
	}
	if info.Action != constant.TaskActionTextGenerate {
		t.Fatalf("action = %q, want %q", info.Action, constant.TaskActionTextGenerate)
	}
}

func TestFireflyMultipartEditUsesImageGenerateAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gemini-3-pro-image")
	_ = writer.WriteField("prompt", "edit this image")
	file, err := writer.CreateFormFile("image", "input.png")
	if err != nil {
		t.Fatalf("create multipart image: %v", err)
	}
	_, _ = file.Write([]byte("\x89PNG\r\n\x1a\n"))
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body.Bytes()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		t.Fatalf("cache multipart body: %v", err)
	}
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind multipart body: %v", err)
	}
	c.Request.Body = io.NopCloser(storage)
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		OriginModelName: "gemini-3-pro-image",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeFirefly,
			UpstreamModelName: "gemini-3-pro-image",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("multipart edit validation failed: %#v", taskErr)
	}
	if info.Action != constant.TaskActionGenerate {
		t.Fatalf("action = %q, want %q", info.Action, constant.TaskActionGenerate)
	}
}

func TestBuildFireflyRequestBodyUsesChatContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	req := relaycommon.TaskSubmitReq{
		Prompt:             "turn this photo into watercolor style",
		Image:              "https://example.com/input.jpg",
		ReferenceImageURLs: []string{"https://example.com/reference.jpg"},
		Mask:               []byte(`"iVBORw0KGgoAAAANSUhEUg=="`),
		Size:               "16x9-2K",
		N:                  2,
	}

	body, err := buildFireflyRequestBody(c, "nanobananapro", req)
	if err != nil {
		t.Fatalf("buildFireflyRequestBody error = %v", err)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	var payload fireflyChatRequest
	if err := common.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal request body error = %v", err)
	}
	if got := payload.Model; got != "firefly-nano-banana-pro-2k-16x9" {
		t.Fatalf("model = %#v; body=%s", got, data)
	}
	if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
		t.Fatalf("messages = %#v; body=%s", payload.Messages, data)
	}
	content := payload.Messages[0].Content
	if len(content) != 4 {
		t.Fatalf("content = %#v; body=%s", content, data)
	}
	if content[0].Type != "text" || content[0].Text != req.Prompt {
		t.Fatalf("text content = %#v", content[0])
	}
	wantImages := []string{req.Image, req.ReferenceImageURLs[0], "data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=="}
	for i, want := range wantImages {
		part := content[i+1]
		if part.Type != "image_url" || part.ImageURL == nil || part.ImageURL.URL != want {
			t.Fatalf("image content[%d] = %#v, want %q", i+1, part, want)
		}
	}
	if payload.N == nil || *payload.N != 2 {
		t.Fatalf("n = %#v, want 2; body=%s", payload.N, data)
	}
	if strings.Contains(string(data), "gemini-3-pro-image") {
		t.Fatalf("request body unexpectedly contains public mapping source: %s", data)
	}
}

func TestDoFireflyResponseStoresImageAndHidesInternalModel(t *testing.T) {
	oldOptions := snapshotOSSOptions()
	defer restoreOSSOptions(oldOptions)

	var uploaded int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/generated/result.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"))
		case r.Method == http.MethodPut:
			uploaded++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	oldMaxFileDownloadMB := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 1
	defer func() { constant.MaxFileDownloadMB = oldMaxFileDownloadMB }()
	service.InitHttpClient()
	setOSSOptions(map[string]string{
		"ImageStorageProvider":     "aliyun_oss",
		"AliyunOssEnabled":         "true",
		"AliyunOssEndpoint":        server.URL,
		"AliyunOssBucket":          "127",
		"AliyunOssAccessKeyId":     "id",
		"AliyunOssAccessKeySecret": "secret",
		"AliyunOssPathPrefix":      "firefly-images",
		"AliyunOssPublicBaseUrl":   "https://cdn.example.com",
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Set("task_request", relaycommon.TaskSubmitReq{Model: "gemini-3-pro-image", N: 1})
	upstreamBody := `{"id":"chatcmpl-internal","model":"firefly-nano-banana-pro-2k-16x9","choices":[{"message":{"content":"![Generated Image](` + server.URL + `/generated/result.png)"}}]}`
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName: "gemini-3-pro-image",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeFirefly,
			ChannelBaseUrl:    server.URL,
			UpstreamModelName: "nanobananapro",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(upstreamBody))}

	_, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse error = %v", taskErr)
	}
	if uploaded != 1 {
		t.Fatalf("uploaded = %d, want 1", uploaded)
	}
	for _, body := range []string{w.Body.String(), string(taskData)} {
		if strings.Contains(body, "firefly-nano") || strings.Contains(body, "nanobananapro") || strings.Contains(body, server.URL) {
			t.Fatalf("public response leaked internal data: %s", body)
		}
		if !strings.Contains(body, `"modelCode":"gemini-3-pro-image"`) || !strings.Contains(body, "https://cdn.example.com/firefly-images/") {
			t.Fatalf("public response missing public model or OSS URL: %s", body)
		}
	}
	initial := adaptor.InitialTaskInfo()
	if initial == nil || initial.Status != model.TaskStatusSuccess || initial.Url == "" {
		t.Fatalf("initial task info = %#v, want completed task", initial)
	}
}

func TestFireflyResponseRejectsCrossOriginResultURL(t *testing.T) {
	oldOptions := snapshotOSSOptions()
	defer restoreOSSOptions(oldOptions)
	var downloaded, uploaded int
	crossOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloaded++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"))
	}))
	defer crossOrigin.Close()
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			uploaded++
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer storage.Close()
	setOSSOptions(map[string]string{
		"ImageStorageProvider":     "aliyun_oss",
		"AliyunOssEnabled":         "true",
		"AliyunOssEndpoint":        storage.URL,
		"AliyunOssBucket":          "127",
		"AliyunOssAccessKeyId":     "id",
		"AliyunOssAccessKeySecret": "secret",
		"AliyunOssPublicBaseUrl":   "https://cdn.example.com",
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Set("task_request", relaycommon.TaskSubmitReq{N: 1})
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName: "gemini-3-pro-image",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeFirefly,
			ChannelBaseUrl: "http://127.0.0.1:6001",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"![Generated Image](` + crossOrigin.URL + `/generated/result.png)"}}]}`)),
	}

	_, _, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr == nil || taskErr.Code != "save_result_file_failed" {
		t.Fatalf("task error = %#v, want safe storage failure", taskErr)
	}
	if downloaded != 0 || uploaded != 0 {
		t.Fatalf("cross-origin result performed I/O: downloaded=%d uploaded=%d", downloaded, uploaded)
	}
	if strings.Contains(taskErr.Message, crossOrigin.URL) || strings.Contains(w.Body.String(), crossOrigin.URL) {
		t.Fatalf("error leaked upstream URL: error=%q body=%s", taskErr.Message, w.Body.String())
	}
}

func TestSanitizeFireflyTextReplacesInternalModels(t *testing.T) {
	input := `models firefly-nano-banana-pro-2k-16x9, firely-nano-banana-2k-1x1, nanobananapro and gpt-image-2 failed`
	got := sanitizeFireflyText(input, "gemini-3-pro-image")
	if strings.Contains(got, "firefly-") || strings.Contains(got, "firely-") || strings.Contains(got, "nanobananapro") || strings.Contains(got, "gpt-image-2") {
		t.Fatalf("sanitizeFireflyText leaked internal model: %s", got)
	}
	if !strings.Contains(got, "gemini-3-pro-image") {
		t.Fatalf("sanitizeFireflyText missing public model: %s", got)
	}
}

func TestFireflyErrorPathsHideMappedModel(t *testing.T) {
	publicModel := "gemini-3-pro-image"
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"message":"model firefly-gpt-image-4k-16x9 / gpt-image-2 failed"}}`,
		)),
	}
	sanitized := sanitizeFireflyErrorResponse(resp, publicModel)
	body, err := io.ReadAll(sanitized.Body)
	if err != nil {
		t.Fatalf("read sanitized error: %v", err)
	}
	assertNoFireflyInternalModel(t, string(body))
	if !strings.Contains(string(body), publicModel) {
		t.Fatalf("sanitized non-2xx error missing public model: %s", body)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName: publicModel,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeFirefly,
			ChannelBaseUrl:    "http://127.0.0.1:6001",
			UpstreamModelName: "gpt-image-2",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	_, _, taskErr := adaptor.DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"message":"firely-gpt-image-4k-16x9 gpt-image-2 failed"}}`,
		)),
	}, info)
	if taskErr == nil {
		t.Fatal("200 error response unexpectedly succeeded")
	}
	assertNoFireflyInternalModel(t, taskErr.Message)
	if !strings.Contains(taskErr.Message, publicModel) {
		t.Fatalf("sanitized 200 error missing public model: %s", taskErr.Message)
	}
}

func assertNoFireflyInternalModel(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, forbidden := range []string{"firefly-", "firely-", "nanobanana", "nano-banana", "gpt-image-2"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("text leaked internal model %q: %s", forbidden, text)
		}
	}
}

func TestValidateFireflyOutputCountRequiresExactBillingCount(t *testing.T) {
	tests := []struct {
		requested int
		actual    int
		wantErr   bool
	}{
		{requested: 0, actual: 1},
		{requested: 1, actual: 1},
		{requested: 2, actual: 2},
		{requested: 0, actual: 2, wantErr: true},
		{requested: 1, actual: 2, wantErr: true},
		{requested: 2, actual: 1, wantErr: true},
	}
	for _, test := range tests {
		err := validateFireflyOutputCount(test.requested, test.actual)
		if (err != nil) != test.wantErr {
			t.Fatalf("validateFireflyOutputCount(%d, %d) error = %v, wantErr=%v", test.requested, test.actual, err, test.wantErr)
		}
	}
}
