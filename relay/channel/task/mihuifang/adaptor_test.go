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

	body := `{"id":"upstream","status":"completed","result":{"image_url":"` + ts.URL + `/a.png","image_urls":["` + ts.URL + `/b.png"],"url":"` + ts.URL + `/c.png"},"url":"` + ts.URL + `/d.png"}`
	ti, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
	if err != nil {
		t.Fatalf("ParseTaskResult error = %v", err)
	}
	if ti.Status != model.TaskStatusSuccess {
		t.Fatalf("status = %s, want %s, reason=%s", ti.Status, model.TaskStatusSuccess, ti.Reason)
	}
	if uploaded != 4 {
		t.Fatalf("uploaded = %d, want 4", uploaded)
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
	if !strings.Contains(string(data), `"model":"nano-banana-pro"`) {
		t.Fatalf("body = %s, want mapped upstream model", string(data))
	}
}

func TestDoResponseUsesOriginModelName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("task_request", relaycommon.TaskSubmitReq{Model: "gemini-3-pro-image"})

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"upstream","status":"pending","model":"nano-banana-pro"}`)),
	}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName: "gemini-3-pro-image",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "nano-banana-pro"},
	}

	_, _, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse taskErr = %v", taskErr)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"model":"gemini-3-pro-image"`) {
		t.Fatalf("response body = %s, want origin model", body)
	}
	if strings.Contains(body, `"model":"nano-banana-pro"`) {
		t.Fatalf("response body leaked upstream model: %s", body)
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
		Data: []byte(`{"id":"upstream","task_id":"upstream","object":"async.generation","type":"image","status":"completed","model":"nano-banana-pro","result":{"image_url":"https://oss.example.com/a.png"}}`),
	}

	body := string(ConvertStoredTask(task))
	if !strings.Contains(body, `"model":"gemini-3-pro-image"`) {
		t.Fatalf("stored response = %s, want origin model", body)
	}
	if strings.Contains(body, `"model":"nano-banana-pro"`) {
		t.Fatalf("stored response leaked upstream model: %s", body)
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

	body := `{"id":"upstream","status":"completed","result":{"image_url":"https://raw.example.com/a.png"}}`
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
		"gemini-3-pro-image@4k@high": 0.08
	}`); err != nil {
		t.Fatalf("UpdateModelPriceByJSONString error = %v", err)
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:      "gemini-3-pro-image",
		Resolution: "4K",
		Quality:    "high",
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
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "nano-banana-pro"},
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
