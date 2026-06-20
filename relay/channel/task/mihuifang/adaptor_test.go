package mihuifang

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
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
