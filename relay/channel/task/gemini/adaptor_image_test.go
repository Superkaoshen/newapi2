package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func TestBuildGeminiImageRequestBodyFromLegacyAsyncRequest(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt: "让这只猫戴上宇航员头盔",
		Images: []string{
			"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=",
			"https://example.com/ref.png",
		},
		Size: "16x9-4K",
		N:    2,
	}

	body, err := buildGeminiImageRequestBody(req)
	if err != nil {
		t.Fatalf("buildGeminiImageRequestBody error = %v", err)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}

	var payload map[string]interface{}
	if err := common.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal body error = %v; body=%s", err, data)
	}
	config := payload["generationConfig"].(map[string]interface{})
	if got := config["responseModalities"].([]interface{})[0]; got != "IMAGE" {
		t.Fatalf("responseModalities[0] = %v, want IMAGE", got)
	}
	imageConfig := config["imageConfig"].(map[string]interface{})
	if got := imageConfig["aspectRatio"]; got != "16:9" {
		t.Fatalf("aspectRatio = %v, want 16:9", got)
	}
	if got := imageConfig["imageSize"]; got != "4K" {
		t.Fatalf("imageSize = %v, want 4K", got)
	}
	if got := config["candidateCount"]; got != float64(2) {
		t.Fatalf("candidateCount = %v, want 2", got)
	}
	contents := payload["contents"].([]interface{})
	parts := contents[0].(map[string]interface{})["parts"].([]interface{})
	if _, ok := parts[0].(map[string]interface{})["inlineData"]; !ok {
		t.Fatalf("first part missing inlineData: %s", data)
	}
	if _, ok := parts[1].(map[string]interface{})["fileData"]; !ok {
		t.Fatalf("second part missing fileData: %s", data)
	}
	if got := parts[2].(map[string]interface{})["text"]; got != req.Prompt {
		t.Fatalf("text part = %v, want prompt", got)
	}
}

func TestGeminiImageBuildRequestURLUsesGenerateContent(t *testing.T) {
	adaptor := &TaskAdaptor{baseURL: "https://api.nanobananai.com"}
	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3-pro-image-preview"},
	})
	if err != nil {
		t.Fatalf("BuildRequestURL error = %v", err)
	}
	if !strings.HasSuffix(url, "/v1beta/models/gemini-3-pro-image-preview:generateContent") {
		t.Fatalf("url = %s, want generateContent endpoint", url)
	}
}

func TestGeminiImagePixelSizeMapsTierAndAspect(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt: "draw",
		Size:   "5504x3072",
	}
	body, err := buildGeminiImageRequestBody(req)
	if err != nil {
		t.Fatalf("buildGeminiImageRequestBody error = %v", err)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	var payload map[string]interface{}
	if err := common.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal body error = %v", err)
	}
	config := payload["generationConfig"].(map[string]interface{})
	imageConfig := config["imageConfig"].(map[string]interface{})
	if got := imageConfig["aspectRatio"]; got != "16:9" {
		t.Fatalf("aspectRatio = %v, want 16:9", got)
	}
	if got := imageConfig["imageSize"]; got != "4K" {
		t.Fatalf("imageSize = %v, want 4K", got)
	}
}

func TestGeminiImageAutoSizeMapsTierWithoutFixedAspect(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt: "draw",
		Size:   "auto-4k",
	}
	body, err := buildGeminiImageRequestBody(req)
	if err != nil {
		t.Fatalf("buildGeminiImageRequestBody error = %v", err)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	var payload map[string]interface{}
	if err := common.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal body error = %v", err)
	}
	config := payload["generationConfig"].(map[string]interface{})
	imageConfig := config["imageConfig"].(map[string]interface{})
	if _, ok := imageConfig["aspectRatio"]; ok {
		t.Fatalf("aspectRatio should be omitted for auto ratio: %v", imageConfig["aspectRatio"])
	}
	if got := imageConfig["imageSize"]; got != "4K" {
		t.Fatalf("imageSize = %v, want 4K", got)
	}
}

func TestGeminiImageDoResponseStoresInitialSuccessTaskInfo(t *testing.T) {
	oldOptions := snapshotGeminiOSSOptions()
	defer setGeminiOSSOptions(oldOptions)
	oldMaxFileDownloadMB := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 1
	defer func() { constant.MaxFileDownloadMB = oldMaxFileDownloadMB }()

	var uploaded int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		uploaded++
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	service.InitHttpClient()
	setGeminiOSSOptions(map[string]string{
		"AliyunOssEnabled":         "true",
		"AliyunOssEndpoint":        ts.URL,
		"AliyunOssBucket":          "127",
		"AliyunOssAccessKeyId":     "id",
		"AliyunOssAccessKeySecret": "secret",
		"AliyunOssPathPrefix":      "async-images",
		"AliyunOssPublicBaseUrl":   "https://cdn.example.com",
	})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-pro-image-preview",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3-pro-image-preview"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{
			"candidates": [{
				"content": {
					"parts": [{
						"inlineData": {
							"mimeType": "image/png",
							"data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII="
						}
					}]
				}
			}]
		}`)),
	}

	adaptor := &TaskAdaptor{}
	upstreamID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse taskErr = %v", taskErr)
	}
	if upstreamID != "task_public" {
		t.Fatalf("upstreamID = %q, want public task id", upstreamID)
	}
	if uploaded != 1 {
		t.Fatalf("uploaded = %d, want 1", uploaded)
	}
	initial := adaptor.InitialTaskInfo()
	if initial == nil {
		t.Fatal("InitialTaskInfo is nil")
	}
	if initial.Status != model.TaskStatusSuccess || initial.Progress != "100%" {
		t.Fatalf("initial status/progress = %s/%s", initial.Status, initial.Progress)
	}
	if !strings.Contains(string(taskData), "https://cdn.example.com/async-images/") {
		t.Fatalf("taskData does not contain OSS URL: %s", taskData)
	}
	if strings.Contains(string(taskData), "iVBOR") {
		t.Fatalf("taskData leaked raw base64: %s", taskData)
	}
}

func snapshotGeminiOSSOptions() map[string]string {
	keys := []string{
		"AliyunOssEnabled",
		"AliyunOssEndpoint",
		"AliyunOssBucket",
		"AliyunOssAccessKeyId",
		"AliyunOssAccessKeySecret",
		"AliyunOssPathPrefix",
		"AliyunOssPublicBaseUrl",
	}
	out := make(map[string]string, len(keys))
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	for _, key := range keys {
		out[key] = common.OptionMap[key]
	}
	return out
}

func setGeminiOSSOptions(values map[string]string) {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	for key, value := range values {
		common.OptionMap[key] = value
	}
}
