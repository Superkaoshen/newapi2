package asyncimage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

type imageRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Images      []string `json:"images,omitempty"`
	AspectRatio string   `json:"aspectRatio,omitempty"`
	ImageSize   string   `json:"imageSize,omitempty"`
	ReplyType   string   `json:"replyType,omitempty"`
}

type imageResult struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Progress int    `json:"progress,omitempty"`
	Results  []struct {
		URL string `json:"url"`
	} `json:"results,omitempty"`
	Error string `json:"error,omitempty"`
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req imageRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	req.Model = strings.TrimSpace(req.Model)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Model == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model is required"), "missing_model", http.StatusBadRequest)
	}
	if req.Prompt == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	if !isSupportedModel(req.Model) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported model: %s", req.Model), "unsupported_model", http.StatusBadRequest)
	}
	if req.ReplyType == "" {
		req.ReplyType = "async"
	}
	if req.ReplyType != "async" && req.ReplyType != "json" && req.ReplyType != "stream" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported replyType: %s", req.ReplyType), "invalid_reply_type", http.StatusBadRequest)
	}
	info.Action = constant.TaskActionAsyncGenerate
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/v1/api/generate", nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	raw, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := raw.(imageRequest)
	if !ok {
		return nil, fmt.Errorf("unexpected task_request type")
	}
	req.Model = info.UpstreamModelName
	req.ReplyType = "async"
	data, err := common.Marshal(req)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var result imageResult
	if err := common.Unmarshal(responseBody, &result); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if result.ID == "" {
		return "", nil, service.TaskErrorWrapperLocal(fmt.Errorf("missing task id"), "invalid_response", http.StatusInternalServerError)
	}
	if result.Status == "failed" || result.Status == "violation" {
		return "", nil, service.TaskErrorWrapperLocal(errors.New(resultError(result)), result.Status, http.StatusBadRequest)
	}

	publicResp := imageResult{
		ID:     info.PublicTaskID,
		Status: "running",
	}
	c.JSON(http.StatusOK, publicResp)
	return result.ID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	requestURL := strings.TrimRight(baseURL, "/") + "/v1/api/result?id=" + taskID
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var result imageResult
	if err := common.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal result failed: %w", err)
	}
	info := &relaycommon.TaskInfo{
		TaskID:   result.ID,
		Progress: progressText(result),
		Reason:   result.Error,
	}
	switch result.Status {
	case "running":
		info.Status = model.TaskStatusInProgress
	case "succeeded":
		info.Status = model.TaskStatusSuccess
		info.Progress = taskcommon.ProgressComplete
		info.Url = firstResultURL(result)
	case "failed", "violation":
		info.Status = model.TaskStatusFailure
		info.Progress = taskcommon.ProgressComplete
		info.Reason = resultError(result)
	default:
		info.Status = model.TaskStatusUnknown
	}
	return info, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"nano-banana",
		"nano-banana-fast",
		"nano-banana-2",
		"nano-banana-2-cl",
		"nano-banana-2-4k-cl",
		"nano-banana-pro",
		"nano-banana-pro-cl",
		"nano-banana-pro-vip",
		"nano-banana-pro-4k-vip",
		"gpt-image-2",
		"gpt-image-2-vip",
	}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "异步图片生成"
}

func isSupportedModel(modelName string) bool {
	for _, item := range (&TaskAdaptor{}).GetModelList() {
		if item == modelName {
			return true
		}
	}
	return false
}

func firstResultURL(result imageResult) string {
	if len(result.Results) == 0 {
		return ""
	}
	return result.Results[0].URL
}

func resultError(result imageResult) string {
	if result.Error != "" {
		return result.Error
	}
	if result.Status != "" {
		return result.Status
	}
	return "task failed"
}

func progressText(result imageResult) string {
	if result.Progress <= 0 {
		return ""
	}
	if result.Progress > 100 {
		result.Progress = 100
	}
	return fmt.Sprintf("%d%%", result.Progress)
}
