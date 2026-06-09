package vectorizer

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
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

const (
	modelVectorizer = "vectorizer"
	formatEPS       = "eps"
	formatSVG       = "svg"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

type vectorizerRequest struct {
	Model     string   `json:"model"`
	Image     string   `json:"image,omitempty"`
	Images    []string `json:"images,omitempty"`
	Format    string   `json:"format,omitempty"`
	ReplyType string   `json:"replyType,omitempty"`
}

type addTaskResponse struct {
	Code    int          `json:"code"`
	Message string       `json:"message,omitempty"`
	ID      scalarString `json:"id,omitempty"`
	TaskID  scalarString `json:"taskid,omitempty"`
}

type tryGetResponse struct {
	Code    int          `json:"code"`
	Message string       `json:"message,omitempty"`
	TaskID  scalarString `json:"taskid,omitempty"`
	URL     string       `json:"url,omitempty"`
}

type publicResponse struct {
	ID       string              `json:"id"`
	Status   string              `json:"status"`
	Progress int                 `json:"progress,omitempty"`
	Results  []map[string]string `json:"results,omitempty"`
	Error    string              `json:"error,omitempty"`
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	a.apiKey = info.ApiKey
	if a.baseURL == "" {
		a.baseURL = constant.ChannelBaseURLs[constant.ChannelTypeVectorizer]
	}
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req vectorizerRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		return service.TaskErrorWrapperLocal(errors.New("model is required"), "missing_model", http.StatusBadRequest)
	}
	if !isSupportedModel(req.Model) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported model: %s", req.Model), "unsupported_model", http.StatusBadRequest)
	}

	req.Image = strings.TrimSpace(req.Image)
	if req.Image == "" && len(req.Images) > 0 {
		req.Image = strings.TrimSpace(req.Images[0])
	}
	if req.Image == "" {
		return service.TaskErrorWrapperLocal(errors.New("image is required"), "invalid_request", http.StatusBadRequest)
	}

	req.Format = normalizeFormat(req.Format)
	if req.ReplyType == "" {
		req.ReplyType = "async"
	}
	if req.ReplyType != "async" && req.ReplyType != "json" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported replyType: %s", req.ReplyType), "invalid_reply_type", http.StatusBadRequest)
	}

	info.Action = constant.TaskActionAsyncGenerate
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/add_task", nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	if contentType, ok := c.Get("vectorizer_content_type"); ok {
		req.Header.Set("Content-Type", contentType.(string))
	}
	req.Header.Set("Accept", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	raw, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := raw.(vectorizerRequest)
	if !ok {
		return nil, fmt.Errorf("unexpected task_request type")
	}

	imageBytes, fileName, err := loadInputImage(req.Image)
	if err != nil {
		return nil, err
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("format", req.Format); err != nil {
		return nil, err
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(imageBytes); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	c.Set("vectorizer_content_type", writer.FormDataContentType())
	info.UpstreamRequestBodySize = int64(body.Len())
	return body, nil
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

	var result addTaskResponse
	if err := common.Unmarshal(responseBody, &result); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if result.Code != 0 {
		return "", nil, service.TaskErrorWrapperLocal(errors.New(resultMessage(result.Message)), "upstream_task_failed", http.StatusBadRequest)
	}

	taskID := strings.TrimSpace(result.ID.String())
	if taskID == "" {
		taskID = strings.TrimSpace(result.TaskID.String())
	}
	if taskID == "" {
		return "", nil, service.TaskErrorWrapperLocal(errors.New("missing task id"), "invalid_response", http.StatusInternalServerError)
	}

	req, _ := c.Get("task_request")
	format := formatEPS
	if vr, ok := req.(vectorizerRequest); ok {
		format = vr.Format
	}

	c.JSON(http.StatusOK, publicResponse{
		ID:     info.PublicTaskID,
		Status: "running",
	})
	return encodeTaskID(taskID, format), responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	encodedTaskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(encodedTaskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	taskID, format := decodeTaskID(encodedTaskID)
	baseURL = strings.TrimRight(baseURL, "/")

	requestURL := baseURL + "/try_get?taskid=" + url.QueryEscape(taskID)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return buildJSONResponse(resp.StatusCode, respBody), nil
	}

	var result tryGetResponse
	if err := common.Unmarshal(respBody, &result); err != nil {
		return buildJSONResponse(http.StatusOK, respBody), nil
	}
	if result.Code != 0 {
		return buildJSONResponse(http.StatusOK, respBody), nil
	}
	if !service.IsAliyunOssEnabled() {
		return buildVectorizerFailureResponse(taskID, "aliyun oss is not enabled"), nil
	}

	downloadURL := baseURL + "/get_image?taskid=" + url.QueryEscape(taskID)
	ossURL, err := service.SaveFileURLToAliyunOSS(downloadURL, "", contentTypeForFormat(format))
	if err != nil {
		return buildVectorizerFailureResponse(taskID, err.Error()), nil
	}

	doneBody, err := common.Marshal(map[string]any{
		"code":   0,
		"taskid": encodedTaskID,
		"url":    ossURL,
		"format": format,
	})
	if err != nil {
		return nil, err
	}
	return buildJSONResponse(http.StatusOK, doneBody), nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var result tryGetResponse
	if err := common.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal result failed: %w", err)
	}

	info := &relaycommon.TaskInfo{
		TaskID:   result.TaskID.String(),
		Progress: taskcommon.ProgressInProgress,
		Reason:   result.Message,
	}
	switch result.Code {
	case 0:
		info.Status = model.TaskStatusSuccess
		info.Progress = taskcommon.ProgressComplete
		info.Url = result.URL
	case -1:
		info.Status = model.TaskStatusFailure
		info.Progress = taskcommon.ProgressComplete
		info.Reason = resultMessage(result.Message)
	default:
		info.Status = model.TaskStatusInProgress
	}
	return info, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{modelVectorizer}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "Vectorizer"
}

func isSupportedModel(modelName string) bool {
	return modelName == modelVectorizer
}

type scalarString string

func (s *scalarString) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*s = ""
		return nil
	}
	if strings.HasPrefix(raw, "\"") {
		var value string
		if err := common.Unmarshal(data, &value); err != nil {
			return err
		}
		*s = scalarString(value)
		return nil
	}
	if isJSONNumberToken(raw) {
		*s = scalarString(raw)
		return nil
	}
	return fmt.Errorf("expected string or number")
}

func (s scalarString) String() string {
	return string(s)
}

func isJSONNumberToken(raw string) bool {
	if raw == "" {
		return false
	}
	first := raw[0]
	if first != '-' && (first < '0' || first > '9') {
		return false
	}
	for i := 1; i < len(raw); i++ {
		ch := raw[i]
		if (ch >= '0' && ch <= '9') || ch == '.' || ch == 'e' || ch == 'E' || ch == '+' || ch == '-' {
			continue
		}
		return false
	}
	return true
}

func normalizeFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case formatSVG:
		return formatSVG
	default:
		return formatEPS
	}
}

func encodeTaskID(taskID string, format string) string {
	return normalizeFormat(format) + ":" + taskID
}

func decodeTaskID(encoded string) (string, string) {
	parts := strings.SplitN(encoded, ":", 2)
	if len(parts) == 2 {
		return parts[1], normalizeFormat(parts[0])
	}
	return encoded, formatEPS
}

func contentTypeForFormat(format string) string {
	if normalizeFormat(format) == formatSVG {
		return "image/svg+xml"
	}
	return "application/postscript"
}

func loadInputImage(input string) ([]byte, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, "", fmt.Errorf("image is empty")
	}
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		mimeType, data, err := service.GetImageFromUrl(input)
		if err != nil {
			return nil, "", err
		}
		imageBytes, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, "", err
		}
		return imageBytes, "image." + imageExt(mimeType), nil
	}

	mimeType, data, err := service.DecodeBase64FileData(input)
	if err != nil {
		return nil, "", err
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, "", fmt.Errorf("input file is not image, content-type=%s", mimeType)
	}
	imageBytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", err
	}
	return imageBytes, "image." + imageExt(mimeType), nil
}

func imageExt(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	case "image/bmp":
		return "bmp"
	default:
		return "png"
	}
}

func resultMessage(message string) string {
	if strings.TrimSpace(message) == "" {
		return "task failed"
	}
	return message
}

func buildVectorizerFailureResponse(taskID string, message string) *http.Response {
	body, _ := common.Marshal(map[string]any{
		"code":    -1,
		"taskid":  taskID,
		"message": resultMessage(message),
	})
	return buildJSONResponse(http.StatusOK, body)
}

func buildJSONResponse(statusCode int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}
