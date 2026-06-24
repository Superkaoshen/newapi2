package mihuifang

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func TestParseTaskResultReplacesResultImagesWithOSSURLs(t *testing.T) {
	oldOptions := snapshotOSSOptions()
	defer restoreOSSOptions(oldOptions)

	var uploaded int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"))
		case http.MethodPut:
			uploaded++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer ts.Close()
	restoreFetch := allowTestServerPort(t, ts.URL)
	defer restoreFetch()
	oldMaxFileDownloadMB := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 1
	defer func() { constant.MaxFileDownloadMB = oldMaxFileDownloadMB }()
	service.InitHttpClient()

	setOSSOptions(map[string]string{
		"AliyunOssEnabled":         "true",
		"AliyunOssEndpoint":        ts.URL,
		"AliyunOssBucket":          "127",
		"AliyunOssAccessKeyId":     "id",
		"AliyunOssAccessKeySecret": "secret",
		"AliyunOssPathPrefix":      "async-images",
		"AliyunOssPublicBaseUrl":   "https://cdn.example.com",
	})

	body := `{"requestId":"upstream","status":"succeeded","result":{"items":[{"url":"` + ts.URL + `/a.png","type":"image"},{"url":"` + ts.URL + `/b.psd","type":"document"}]}}`
	ti, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
	if err != nil {
		t.Fatalf("ParseTaskResult error = %v", err)
	}
	if ti.Status != model.TaskStatusSuccess {
		t.Fatalf("status = %s, want %s, reason=%s", ti.Status, model.TaskStatusSuccess, ti.Reason)
	}
	if uploaded != 2 {
		t.Fatalf("uploaded = %d, want 2", uploaded)
	}
	data := string(ti.Data)
	if strings.Contains(data, ts.URL) {
		t.Fatalf("response data leaked upstream url: %s", data)
	}
	if !strings.Contains(data, "https://cdn.example.com/async-images/") {
		t.Fatalf("response data does not contain OSS URL: %s", data)
	}
}

func TestBuildRequestBodyAllowsMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "gemini-3-pro-image",
		Prompt: "draw a cat",
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "nano-banana-pro"},
	}

	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody error = %v", err)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	if !strings.Contains(string(data), `"model":"nanobananapro"`) {
		t.Fatalf("body = %s, want mapped upstream model", string(data))
	}
}

func TestBuildRequestBodyAllowsProVariantModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "gemini-3-pro-image",
		Prompt: "draw a cat",
		Size:   "16x9-4K",
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "nanobananapro-5"},
	}

	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody error = %v", err)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	var payload map[string]interface{}
	if err := common.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal body error = %v", err)
	}
	if got := payload["model"]; got != "nanobananapro-5" {
		t.Fatalf("model = %v, want nanobananapro-5; body=%s", got, data)
	}
	if got := payload["size"]; got != "5504x3072" {
		t.Fatalf("size = %v, want 5504x3072; body=%s", got, data)
	}
	if got := payload["quality"]; got != "hd" {
		t.Fatalf("quality = %v, want hd; body=%s", got, data)
	}
}

func TestBuildRequestBodyNormalizesNanoLegacySize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "nanobanana2",
		Prompt: "draw a poster",
		Size:   "9x16-4k",
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "nanobanana2"},
	}

	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody error = %v", err)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	var payload map[string]interface{}
	if err := common.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal body error = %v", err)
	}
	if got := payload["size"]; got != "3072x5504" {
		t.Fatalf("size = %v, want 3072x5504; body=%s", got, data)
	}
	if got := payload["quality"]; got != "hd" {
		t.Fatalf("quality = %v, want hd; body=%s", got, data)
	}
	if strings.Contains(string(data), "9x16-4k") {
		t.Fatalf("body leaked legacy size: %s", data)
	}
}

func TestBuildRequestBodyUsesOfficialNanoSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "nanobanana",
		Prompt: "draw a product",
		Size:   "1584x672",
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "nanobanana"},
	}

	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("BuildRequestBody error = %v", err)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	var payload map[string]interface{}
	if err := common.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal body error = %v", err)
	}
	if got := payload["size"]; got != "1584x672" {
		t.Fatalf("size = %v, want 1584x672; body=%s", got, data)
	}
	if got := payload["quality"]; got != "standard" {
		t.Fatalf("quality = %v, want standard; body=%s", got, data)
	}
}

func TestBuildRequestBodyRejectsUnsupportedNanoExtendedRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "nanobananapro",
		Prompt: "draw a banner",
		Size:   "8x1-1k",
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "nanobananapro"},
	}

	if _, err := (&TaskAdaptor{}).BuildRequestBody(c, info); err == nil || !strings.Contains(err.Error(), "does not support ratio 8:1") {
		t.Fatalf("BuildRequestBody error = %v, want unsupported extended ratio", err)
	}
}

func TestBuildRequestBodyEnforcesImageLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "nanobanana",
		Prompt: "draw",
		Images: []string{
			"https://example.com/1.png",
			"https://example.com/2.png",
			"https://example.com/3.png",
			"https://example.com/4.png",
			"https://example.com/5.png",
		},
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "nanobanana"},
	}

	if _, err := (&TaskAdaptor{}).BuildRequestBody(c, info); err == nil || !strings.Contains(err.Error(), "at most 4 input images") {
		t.Fatalf("BuildRequestBody error = %v, want image limit error", err)
	}
}

func TestTaskSubmitReqAcceptsImageArray(t *testing.T) {
	var req relaycommon.TaskSubmitReq
	if err := common.Unmarshal([]byte(`{"model":"nanobanana","prompt":"draw","image":["https://example.com/a.png","https://example.com/b.png"]}`), &req); err != nil {
		t.Fatalf("Unmarshal TaskSubmitReq error = %v", err)
	}
	if len(req.Images) != 2 {
		t.Fatalf("images = %#v, want 2 items", req.Images)
	}
	if req.Image != "https://example.com/a.png" {
		t.Fatalf("image = %q, want first array item", req.Image)
	}
}

func TestDoResponseUsesOriginModelName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("task_request", relaycommon.TaskSubmitReq{Model: "gemini-3-pro-image"})

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"requestId":"upstream","status":"submitted","modelCode":"nanobananapro","modelName":"frefly-nano-banana-1k-1x1"}`)),
	}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName: "gemini-3-pro-image",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "nano-banana-pro"},
	}

	_, taskData, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse taskErr = %v", taskErr)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"modelCode":"gemini-3-pro-image"`) {
		t.Fatalf("response body = %s, want origin model", body)
	}
	if strings.Contains(body, `"modelCode":"nanobananapro"`) {
		t.Fatalf("response body leaked upstream model: %s", body)
	}
	assertNoUpstreamModelName(t, body)
	assertNoUpstreamModelName(t, string(taskData))
	if !strings.Contains(body, `"requestId":"task_public"`) {
		t.Fatalf("response body = %s, want public requestId", body)
	}
}

func TestDoResponseAcceptsStringTaskOrderID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("task_request", relaycommon.TaskSubmitReq{Model: "nanobanana"})

	resp := &http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(strings.NewReader(`{"taskOrderId":"2069392637227798530","requestId":"upstream","status":"created","billingStatus":"pending","progress":0}`)),
	}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName: "nanobanana",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "nanobanana"},
	}

	_, taskData, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse taskErr = %v", taskErr)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"taskOrderId":"2069392637227798530"`) {
		t.Fatalf("response body = %s, want string taskOrderId preserved", body)
	}
	if !strings.Contains(string(taskData), `"taskOrderId":"2069392637227798530"`) {
		t.Fatalf("task data = %s, want string taskOrderId preserved", taskData)
	}
}

func TestParseTaskResultDropsUpstreamModelName(t *testing.T) {
	body := `{"requestId":"upstream","status":"processing","modelCode":"nanobananapro","modelName":"frefly-nano-banana-1k-1x1","progress":25}`
	ti, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
	if err != nil {
		t.Fatalf("ParseTaskResult error = %v", err)
	}
	if ti.Status != model.TaskStatusInProgress {
		t.Fatalf("status = %s, want %s", ti.Status, model.TaskStatusInProgress)
	}
	assertNoUpstreamModelName(t, string(ti.Data))
}

func TestParseTaskResultTreatsCreatedAsQueued(t *testing.T) {
	body := `{"requestId":"upstream","status":"created","billingStatus":"pending","progress":0}`
	ti, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
	if err != nil {
		t.Fatalf("ParseTaskResult error = %v", err)
	}
	if ti.Status != model.TaskStatusQueued {
		t.Fatalf("status = %s, want %s", ti.Status, model.TaskStatusQueued)
	}
}

func TestDoResponseOSSFailureDoesNotReturnRawURL(t *testing.T) {
	oldOptions := snapshotOSSOptions()
	defer restoreOSSOptions(oldOptions)
	setOSSOptions(map[string]string{"AliyunOssEnabled": "false"})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("task_request", relaycommon.TaskSubmitReq{Model: "nanobanana"})

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"requestId":"upstream","status":"succeeded","result":{"items":[{"url":"https://raw.example.com/a.png","type":"image"}]}}`)),
	}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName: "nanobanana",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "nanobanana"},
	}

	_, _, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, info)
	if taskErr == nil {
		t.Fatalf("DoResponse taskErr = nil, want OSS failure")
	}
	if strings.Contains(w.Body.String(), "raw.example.com") {
		t.Fatalf("response leaked upstream URL: %s", w.Body.String())
	}
}

func TestConvertStoredTaskUsesOriginModelName(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName:   "gemini-3-pro-image",
			UpstreamModelName: "nano-banana-pro",
		},
		Data: []byte(`{"requestId":"upstream","status":"succeeded","modelCode":"nanobananapro","modelName":"frefly-nano-banana-1k-1x1","result":{"items":[{"url":"https://oss.example.com/a.png","type":"image"}]}}`),
	}

	body := string(ConvertStoredTask(task))
	if !strings.Contains(body, `"modelCode":"gemini-3-pro-image"`) {
		t.Fatalf("stored response = %s, want origin model", body)
	}
	if strings.Contains(body, `"modelCode":"nanobananapro"`) {
		t.Fatalf("stored response leaked upstream model: %s", body)
	}
	assertNoUpstreamModelName(t, body)
	if !strings.Contains(body, `"requestId":"task_public"`) {
		t.Fatalf("stored response = %s, want public requestId", body)
	}
}

func assertNoUpstreamModelName(t *testing.T, body string) {
	t.Helper()
	if strings.Contains(body, `"modelName"`) || strings.Contains(body, "frefly-nano-banana-1k-1x1") {
		t.Fatalf("response leaked upstream modelName: %s", body)
	}
}

func allowTestServerPort(t *testing.T, rawURL string) func() {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	setting := system_setting.GetFetchSetting()
	oldAllowPrivateIP := setting.AllowPrivateIp
	oldPorts := append([]string(nil), setting.AllowedPorts...)
	setting.AllowPrivateIp = true
	setting.AllowedPorts = append(setting.AllowedPorts, u.Port())
	return func() {
		setting.AllowPrivateIp = oldAllowPrivateIP
		setting.AllowedPorts = oldPorts
	}
}

func TestParseTaskResultOSSFailureMarksTaskFailureWithoutRawURL(t *testing.T) {
	oldOptions := snapshotOSSOptions()
	defer restoreOSSOptions(oldOptions)
	setOSSOptions(map[string]string{"AliyunOssEnabled": "false"})

	body := `{"requestId":"upstream","status":"succeeded","result":{"items":[{"url":"https://raw.example.com/a.png","type":"image"}]}}`
	ti, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
	if err != nil {
		t.Fatalf("ParseTaskResult error = %v", err)
	}
	if ti.Status != model.TaskStatusFailure {
		t.Fatalf("status = %s, want %s", ti.Status, model.TaskStatusFailure)
	}
	if !strings.Contains(ti.Reason, "aliyun oss") {
		t.Fatalf("reason = %q, want aliyun oss error", ti.Reason)
	}
	data := string(ti.Data)
	if strings.Contains(data, "raw.example.com") {
		t.Fatalf("failure data leaked upstream url: %s", data)
	}
}

func TestImageSizeTier(t *testing.T) {
	tests := []struct {
		size       string
		resolution string
		want       string
	}{
		{size: "1x1", want: "1k"},
		{size: "16x9-2k", want: "2k"},
		{size: "2k-16x9", want: "2k"},
		{size: "1584x672", want: "1k"},
		{size: "3168x1344", want: "2k"},
		{size: "6336x2688", want: "4k"},
		{size: "3072x384", want: "1k"},
		{size: "3840x2160", want: "4k"},
		{resolution: "4K", want: "4k"},
	}
	for _, tt := range tests {
		if got := imageSizeTier(tt.size, tt.resolution); got != tt.want {
			t.Fatalf("imageSizeTier(%q, %q) = %q, want %q", tt.size, tt.resolution, got, tt.want)
		}
	}
}

func TestEstimateBillingUsesMappedModelResolutionQualityAndCount(t *testing.T) {
	oldPriceJSON := ratio_setting.ModelPrice2JSONString()
	defer func() { _ = ratio_setting.UpdateModelPriceByJSONString(oldPriceJSON) }()
	if err := ratio_setting.UpdateModelPriceByJSONString(`{
		"gemini-3-pro-image": 0.01,
		"gemini-3-pro-image@high@psd": 0.08
	}`); err != nil {
		t.Fatalf("UpdateModelPriceByJSONString error = %v", err)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:      "gemini-3-pro-image",
		Resolution: "4K",
		Quality:    "high",
		OutputPSD:  common.GetPointer(true),
		N:          2,
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-pro-image",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-image-2"},
	}

	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
	if ratios["price_tier"] != 8 {
		t.Fatalf("price_tier ratio = %v, want 8", ratios["price_tier"])
	}
	if _, ok := ratios["quality"]; ok {
		t.Fatalf("quality ratio should be folded into price_tier: %v", ratios)
	}
	if ratios["n"] != 2 {
		t.Fatalf("n ratio = %v, want 2", ratios["n"])
	}
}

func TestEstimateBillingFallsBackToMappedUpstreamPrice(t *testing.T) {
	oldPriceJSON := ratio_setting.ModelPrice2JSONString()
	defer func() { _ = ratio_setting.UpdateModelPriceByJSONString(oldPriceJSON) }()
	if err := ratio_setting.UpdateModelPriceByJSONString(`{
		"nanobanana": 0.04,
		"nanobanana@4k": 0.12
	}`); err != nil {
		t.Fatalf("UpdateModelPriceByJSONString error = %v", err)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:      "public-nano",
		Resolution: "4K",
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "public-nano",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "nanobanana"},
	}

	if got := (&TaskAdaptor{}).ResolveBillingModelName(info); got != "nanobanana" {
		t.Fatalf("ResolveBillingModelName = %q, want nanobanana", got)
	}
	if err := (&TaskAdaptor{}).ValidateBilling(c, info); err != nil {
		t.Fatalf("ValidateBilling error = %v", err)
	}
	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
	if ratios["price_tier"] != 3 {
		t.Fatalf("price_tier ratio = %v, want 3", ratios["price_tier"])
	}
}

func TestEstimateBillingFallsBackFromVariantToModelFamilyPrice(t *testing.T) {
	oldPriceJSON := ratio_setting.ModelPrice2JSONString()
	defer func() { _ = ratio_setting.UpdateModelPriceByJSONString(oldPriceJSON) }()
	if err := ratio_setting.UpdateModelPriceByJSONString(`{
		"nanobananapro": 0.08,
		"nanobananapro@4k": 0.20
	}`); err != nil {
		t.Fatalf("UpdateModelPriceByJSONString error = %v", err)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model: "gemini-3-pro-image",
		Size:  "16x9-4K",
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-pro-image",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "nanobananapro-5"},
	}

	if got := (&TaskAdaptor{}).ResolveBillingModelName(info); got != "nanobananapro" {
		t.Fatalf("ResolveBillingModelName = %q, want nanobananapro", got)
	}
	if err := (&TaskAdaptor{}).ValidateBilling(c, info); err != nil {
		t.Fatalf("ValidateBilling error = %v", err)
	}
	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
	if ratios["price_tier"] != 2.5 {
		t.Fatalf("price_tier ratio = %v, want 2.5", ratios["price_tier"])
	}
}

func TestDefaultMihuifangPricesAreDoubled(t *testing.T) {
	defaultPrices := ratio_setting.GetDefaultModelPriceMap()
	tests := map[string]float64{
		"gpt-image-2":        0.1,
		"gpt-image-2@high":   0.15,
		"gpt-image-2@psd":    0.3,
		"nanobanana":         0.04,
		"nanobanana@4k":      0.12,
		"nanobanana2@2k":     0.1,
		"nanobananapro@4k":   0.2,
		"nano-banana-pro@4k": 0.2,
	}
	for modelName, want := range tests {
		got, ok := defaultPrices[modelName]
		if !ok {
			t.Fatalf("default price for %s is missing", modelName)
		}
		if got != want {
			t.Fatalf("default price for %s = %v, want %v", modelName, got, want)
		}
	}
}

func TestValidateBillingRequiresTierPrice(t *testing.T) {
	oldPriceJSON := ratio_setting.ModelPrice2JSONString()
	defer func() { _ = ratio_setting.UpdateModelPriceByJSONString(oldPriceJSON) }()
	if err := ratio_setting.UpdateModelPriceByJSONString(`{"gemini-3-pro-image": 0.01}`); err != nil {
		t.Fatalf("UpdateModelPriceByJSONString error = %v", err)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:      "gemini-3-pro-image",
		Resolution: "4K",
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-pro-image",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "unpriced-upstream"},
	}

	err := (&TaskAdaptor{}).ValidateBilling(c, info)
	if err == nil || !strings.Contains(err.Error(), "gemini-3-pro-image@4k") {
		t.Fatalf("ValidateBilling error = %v, want missing tier price", err)
	}
}

func snapshotOSSOptions() map[string]string {
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

func restoreOSSOptions(values map[string]string) {
	setOSSOptions(values)
}

func setOSSOptions(values map[string]string) {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	for key, value := range values {
		common.OptionMap[key] = value
	}
}
