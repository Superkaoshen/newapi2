package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
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

func TestGeminiAsyncImageMappedModelUsesGenerateContent(t *testing.T) {
	adaptor := &TaskAdaptor{baseURL: "https://api.nanobananai.com"}
	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeAsyncImageSubmit,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "nano-banana-pro"},
	})
	if err != nil {
		t.Fatalf("BuildRequestURL error = %v", err)
	}
	if !strings.HasSuffix(url, "/v1beta/models/nano-banana-pro:generateContent") {
		t.Fatalf("url = %s, want mapped model generateContent endpoint", url)
	}
}

func TestGeminiImagePixelSizeMapsAspectWithoutImplicitTier(t *testing.T) {
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
	if _, ok := imageConfig["imageSize"]; ok {
		t.Fatalf("imageSize should be omitted when resolution is not explicit: %v", imageConfig["imageSize"])
	}
}

func TestGeminiImageResolutionMapsTier(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:     "draw",
		Size:       "16x9",
		Resolution: "5504x3072",
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

func TestGeminiImageOneByOneSizeOmitsImplicitTier(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt: "draw",
		Size:   "1x1",
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
	if got := imageConfig["aspectRatio"]; got != "1:1" {
		t.Fatalf("aspectRatio = %v, want 1:1", got)
	}
	if _, ok := imageConfig["imageSize"]; ok {
		t.Fatalf("imageSize should be omitted when resolution is not explicit: %v", imageConfig["imageSize"])
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

	responseBody := []byte(`{
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
		}`)

	taskInfo, taskData, err := parseGeminiImageCompletion("task_public", "gemini-3-pro-image-preview", responseBody)
	if err != nil {
		t.Fatalf("parseGeminiImageCompletion error = %v", err)
	}
	if uploaded != 1 {
		t.Fatalf("uploaded = %d, want 1", uploaded)
	}
	if taskInfo.Status != model.TaskStatusSuccess || taskInfo.Progress != "100%" {
		t.Fatalf("task status/progress = %s/%s", taskInfo.Status, taskInfo.Progress)
	}
	if !strings.Contains(string(taskData), "https://cdn.example.com/async-images/") {
		t.Fatalf("taskData does not contain OSS URL: %s", taskData)
	}
	if strings.Contains(string(taskData), "iVBOR") {
		t.Fatalf("taskData leaked raw base64: %s", taskData)
	}
}

func TestParseGeminiImageCompletionRequiresOSSForRemoteURL(t *testing.T) {
	oldOptions := snapshotGeminiOSSOptions()
	defer setGeminiOSSOptions(oldOptions)
	setGeminiOSSOptions(map[string]string{"AliyunOssEnabled": "false"})

	responseBody := []byte(`{
		"candidates": [
			{
				"content": {
					"role": "model",
					"parts": [
						{
							"text": "![generated](https://file5.aitohumanize.com/file/e312fb3496f74d8c947d2d5450cebb6b.png)"
						},
						{
							"fileData": {
								"mimeType": "image/png",
								"fileUri": "https://file5.aitohumanize.com/file/e312fb3496f74d8c947d2d5450cebb6b.png"
							}
						}
					]
				},
				"finishReason": "STOP",
				"index": 0
			}
		],
		"usageMetadata": {
			"promptTokenCount": 12,
			"candidatesTokenCount": 43,
			"totalTokenCount": 55
		},
		"modelVersion": "nano-banana-pro"
	}`)

	_, _, err := parseGeminiImageCompletion("task_public", "gemini-3-pro-image", responseBody)
	if err == nil || !strings.Contains(err.Error(), "aliyun oss") {
		t.Fatalf("parseGeminiImageCompletion error = %v, want aliyun oss error", err)
	}
}

func TestParseGeminiImageCompletionSavesFileDataURLToOSS(t *testing.T) {
	oldOptions := snapshotGeminiOSSOptions()
	defer setGeminiOSSOptions(oldOptions)
	oldMaxFileDownloadMB := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 1
	defer func() { constant.MaxFileDownloadMB = oldMaxFileDownloadMB }()
	fetchSetting := system_setting.GetFetchSetting()
	oldAllowPrivateIP := fetchSetting.AllowPrivateIp
	oldAllowedPorts := append([]string(nil), fetchSetting.AllowedPorts...)
	fetchSetting.AllowPrivateIp = true
	fetchSetting.AllowedPorts = []string{"1-65535"}
	defer func() {
		fetchSetting.AllowPrivateIp = oldAllowPrivateIP
		fetchSetting.AllowedPorts = oldAllowedPorts
	}()

	var uploaded int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{
				0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
				0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
				0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
				0x08, 0x04, 0x00, 0x00, 0x00, 0xb5, 0x1c, 0x0c,
				0x02, 0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41,
				0x54, 0x78, 0xda, 0x63, 0xfc, 0xff, 0x1f, 0x00,
				0x03, 0x03, 0x02, 0x00, 0xef, 0xbf, 0xa7, 0xdb,
				0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44,
				0xae, 0x42, 0x60, 0x82,
			})
		case http.MethodPut:
			uploaded++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
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

	sourceURL := ts.URL + "/source.png"
	responseBody := []byte(`{
		"candidates": [{
			"content": {
				"parts": [
					{
						"text": "![generated](` + sourceURL + `)"
					},
					{
						"fileData": {
							"mimeType": "image/png",
							"fileUri": "` + sourceURL + `"
						}
					}
				]
			}
		}]
	}`)

	taskInfo, taskData, err := parseGeminiImageCompletion("task_public", "gemini-3-pro-image", responseBody)
	if err != nil {
		t.Fatalf("parseGeminiImageCompletion error = %v", err)
	}
	if uploaded != 1 {
		t.Fatalf("uploaded = %d, want 1", uploaded)
	}
	if !strings.HasPrefix(taskInfo.Url, "https://cdn.example.com/async-images/") {
		t.Fatalf("taskInfo.Url = %s, want CDN URL", taskInfo.Url)
	}
	body := string(taskData)
	if strings.Contains(body, sourceURL) {
		t.Fatalf("taskData leaked upstream URL: %s", body)
	}
	if strings.Contains(body, "image_urls") {
		t.Fatalf("duplicate markdown/fileData URL should only produce one result: %s", body)
	}
	if !strings.Contains(body, "https://cdn.example.com/async-images/") {
		t.Fatalf("taskData does not contain CDN URL: %s", body)
	}
}

func TestTryResumeImageTaskSkipsInProgressTask(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusInProgress,
		StartTime: time.Now().Unix(),
		PrivateData: model.TaskPrivateData{
			RequestBody: "encoded-request-body",
		},
	}

	if TryResumeImageTask(task, "https://example.com", "sk-test", "") {
		t.Fatal("TryResumeImageTask should skip recent in-progress Gemini image task")
	}
}

func TestGeminiImageDoResponseReturnsSubmittedWithoutWaiting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-pro-image-preview",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3-pro-image-preview"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}
	resp := newGeminiImageSubmittedHTTPResponse("task_public")

	adaptor := &TaskAdaptor{}
	upstreamID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse taskErr = %v", taskErr)
	}
	if upstreamID != "task_public" {
		t.Fatalf("upstreamID = %q, want public task id", upstreamID)
	}
	initial := adaptor.InitialTaskInfo()
	if initial == nil {
		t.Fatal("InitialTaskInfo is nil")
	}
	if initial.Status != model.TaskStatusSubmitted || initial.Progress != "10%" {
		t.Fatalf("initial status/progress = %s/%s", initial.Status, initial.Progress)
	}
	if !strings.Contains(w.Body.String(), `"status":"submitted"`) {
		t.Fatalf("response body = %s, want submitted", w.Body.String())
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
