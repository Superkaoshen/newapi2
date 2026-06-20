package mihuifang

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

type asyncGenerationResponse struct {
	ID        string                 `json:"id,omitempty"`
	TaskID    string                 `json:"task_id,omitempty"`
	Object    string                 `json:"object,omitempty"`
	Type      string                 `json:"type,omitempty"`
	Mode      string                 `json:"mode,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Progress  int                    `json:"progress,omitempty"`
	Model     string                 `json:"model,omitempty"`
	CreatedAt int64                  `json:"created_at,omitempty"`
	Result    map[string]interface{} `json:"result,omitempty"`
	URL       string                 `json:"url,omitempty"`
	Detail    struct {
		Status string `json:"status,omitempty"`
	} `json:"detail,omitempty"`
	Error *struct {
		Message string `json:"message,omitempty"`
		Code    string `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if err := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate); err != nil {
		return err
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest)
	}
	if hasImageInput(req) && req.Mode == "" {
		info.Action = constant.TaskActionGenerate
	} else {
		info.Action = constant.TaskActionTextGenerate
	}
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	priceRatio := imageTierPriceRatio(info.OriginModelName, info.UpstreamModelName, req)
	return map[string]float64{
		"price_tier": priceRatio,
		"n":          imageCountRatio(req.N),
	}
}

func (a *TaskAdaptor) ValidateBilling(c *gin.Context, info *relaycommon.RelayInfo) error {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return err
	}
	if _, ok := lookupConfiguredModelPrice(info.OriginModelName); !ok {
		return fmt.Errorf("model price is required for %s", info.OriginModelName)
	}
	tierKey := imageTierPriceKey(req, info.UpstreamModelName)
	if _, ok := lookupConfiguredModelPrice(info.OriginModelName + tierKey); !ok {
		return fmt.Errorf("model price is required for %s", info.OriginModelName+tierKey)
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return strings.TrimRight(a.baseURL, "/") + "/v1/async/generations", nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	if !isSupportedModel(info.UpstreamModelName) {
		return nil, fmt.Errorf("unsupported model: %s", info.UpstreamModelName)
	}
	body := map[string]interface{}{
		"model":  info.UpstreamModelName,
		"mode":   resolveMode(req),
		"prompt": req.Prompt,
	}
	setString(body, "image", req.Image)
	if len(req.Images) > 0 {
		body["images"] = req.Images
	}
	if len(req.ReferenceImages) > 0 {
		body["referenceImages"] = req.ReferenceImages
	}
	setString(body, "size", req.Size)
	setString(body, "quality", req.Quality)
	setString(body, "aspect_ratio", req.AspectRatio)
	setString(body, "resolution", req.Resolution)
	if req.N > 0 {
		body["n"] = req.N
	}
	data, err := common.Marshal(body)
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

	var upstream asyncGenerationResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}

	upstreamID := firstNonEmpty(upstream.ID, upstream.TaskID)
	if upstreamID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	upstream.ID = info.PublicTaskID
	upstream.TaskID = info.PublicTaskID
	upstream.Object = "async.generation"
	upstream.Type = "image"
	if upstream.Mode == "" {
		req, _ := relaycommon.GetTaskRequest(c)
		upstream.Mode = resolveMode(req)
	}
	if upstream.Status == "" {
		upstream.Status = "pending"
	}
	upstream.Detail.Status = upstream.Status

	c.JSON(http.StatusOK, upstream)
	return upstreamID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	uri := fmt.Sprintf("%s/v1/async/generations/%s", strings.TrimRight(baseUrl, "/"), taskID)
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var resp asyncGenerationResponse
	if err := common.Unmarshal(respBody, &resp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	ti := &relaycommon.TaskInfo{TaskID: firstNonEmpty(resp.ID, resp.TaskID)}
	status := strings.ToLower(firstNonEmpty(resp.Status, resp.Detail.Status))
	switch status {
	case "pending", "queued", "submitted":
		ti.Status = model.TaskStatusQueued
	case "processing", "in_progress", "running":
		ti.Status = model.TaskStatusInProgress
	case "completed", "succeeded", "success":
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
		saved, err := saveResultImagesToOSS(resp.Result, resp.URL)
		if err != nil {
			ti.Status = model.TaskStatusFailure
			ti.Reason = fmt.Sprintf("save result image to aliyun oss failed: %s", err.Error())
			ti.Progress = "100%"
			ti.Data = failureResponseData(resp, ti.Reason)
			return ti, nil
		}
		resp = applySavedImages(resp, saved)
		ti.Url = firstNonEmpty(resp.URL, firstResultImage(resp.Result))
		ti.Data = normalizeResponseData(resp)
	case "failed", "failure", "cancelled", "canceled":
		ti.Status = model.TaskStatusFailure
		ti.Progress = "100%"
		if resp.Error != nil && resp.Error.Message != "" {
			ti.Reason = resp.Error.Message
		} else {
			ti.Reason = "task failed"
		}
	default:
		ti.Status = model.TaskStatusInProgress
	}
	if ti.Progress == "" && resp.Progress > 0 {
		ti.Progress = fmt.Sprintf("%d%%", resp.Progress)
	}
	if ti.Data == nil {
		ti.Data = normalizeResponseData(resp)
	}
	return ti, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func isSupportedModel(modelName string) bool {
	for _, m := range ModelList {
		if modelName == m {
			return true
		}
	}
	return false
}

func hasImageInput(req relaycommon.TaskSubmitReq) bool {
	return strings.TrimSpace(req.Image) != "" || len(req.Images) > 0 || len(req.ReferenceImages) > 0
}

func resolveMode(req relaycommon.TaskSubmitReq) string {
	if strings.TrimSpace(req.Mode) != "" {
		return req.Mode
	}
	if hasImageInput(req) {
		return "image_to_image"
	}
	return "text_to_image"
}

func setString(m map[string]interface{}, key, value string) {
	if strings.TrimSpace(value) != "" {
		m[key] = value
	}
}

func saveResultImagesToOSS(result map[string]interface{}, topURL string) ([]string, error) {
	raw := collectResultImageURLs(result, topURL)
	if len(raw) == 0 {
		return nil, fmt.Errorf("completed response contains no image url")
	}
	saved := make([]string, 0, len(raw))
	for _, u := range raw {
		ossURL, err := service.StrictSaveImageURLToAliyunOSS(u, "")
		if err != nil {
			return nil, err
		}
		saved = append(saved, ossURL)
	}
	return saved, nil
}

func collectResultImageURLs(result map[string]interface{}, topURL string) []string {
	urls := make([]string, 0)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" {
			urls = append(urls, v)
		}
	}
	if result != nil {
		if v, ok := result["image_url"].(string); ok {
			add(v)
		}
		switch v := result["image_urls"].(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					add(s)
				}
			}
		case []string:
			for _, s := range v {
				add(s)
			}
		}
		if v, ok := result["url"].(string); ok {
			add(v)
		}
	}
	add(topURL)
	return uniqueStrings(urls)
}

func applySavedImages(resp asyncGenerationResponse, saved []string) asyncGenerationResponse {
	if resp.Result == nil {
		resp.Result = map[string]interface{}{}
	}
	if len(saved) > 0 {
		resp.Result["image_url"] = saved[0]
		resp.Result["url"] = saved[0]
		resp.URL = saved[0]
	}
	if len(saved) > 1 {
		resp.Result["image_urls"] = saved
	} else {
		delete(resp.Result, "image_urls")
	}
	resp.Object = "async.generation"
	resp.Type = "image"
	resp.Status = "completed"
	resp.Progress = 100
	resp.Detail.Status = "completed"
	return resp
}

func firstResultImage(result map[string]interface{}) string {
	if result == nil {
		return ""
	}
	if v, ok := result["image_url"].(string); ok {
		return v
	}
	if v, ok := result["url"].(string); ok {
		return v
	}
	if arr, ok := result["image_urls"].([]interface{}); ok && len(arr) > 0 {
		if s, ok := arr[0].(string); ok {
			return s
		}
	}
	return ""
}

func normalizeResponseData(resp asyncGenerationResponse) []byte {
	resp.Object = "async.generation"
	resp.Type = "image"
	if resp.Detail.Status == "" {
		resp.Detail.Status = resp.Status
	}
	data, _ := common.Marshal(resp)
	return data
}

func failureResponseData(resp asyncGenerationResponse, reason string) []byte {
	resp.Result = nil
	resp.URL = ""
	resp.Object = "async.generation"
	resp.Type = "image"
	resp.Status = "failed"
	resp.Progress = 100
	resp.Detail.Status = "failed"
	resp.Error = &struct {
		Message string `json:"message,omitempty"`
		Code    string `json:"code,omitempty"`
	}{Message: reason, Code: "save_oss_failed"}
	data, _ := common.Marshal(resp)
	return data
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func imageQualityRatio(modelName, quality string) float64 {
	if modelName != "gpt-image-2" {
		return 1
	}
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "low":
		return 1
	case "high":
		return 3
	case "medium", "":
		return 2
	default:
		return 1
	}
}

func imageTierPriceRatio(originModel, upstreamModel string, req relaycommon.TaskSubmitReq) float64 {
	basePrice, ok := lookupConfiguredModelPrice(originModel)
	if !ok || basePrice <= 0 {
		return 1
	}
	tierKey := imageTierPriceKey(req, upstreamModel)
	tierPrice, ok := lookupConfiguredModelPrice(originModel + tierKey)
	if !ok || tierPrice <= 0 {
		return 1
	}
	return tierPrice / basePrice
}

func imageTierPriceKey(req relaycommon.TaskSubmitReq, upstreamModel string) string {
	parts := []string{imageSizeTier(req.Size, req.Resolution)}
	if upstreamModel == "gpt-image-2" {
		quality := strings.ToLower(strings.TrimSpace(req.Quality))
		if quality == "" {
			quality = "medium"
		}
		parts = append(parts, quality)
	}
	return "@" + strings.Join(parts, "@")
}

func lookupConfiguredModelPrice(names ...string) (float64, bool) {
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if price, ok := ratio_setting.GetModelPrice(name, false); ok {
			return price, true
		}
		if price, ok := ratio_setting.GetDefaultModelPriceMap()[name]; ok {
			return price, true
		}
	}
	return 0, false
}

func imageCountRatio(n int) float64 {
	if n <= 1 {
		return 1
	}
	return float64(n)
}

func imageSizeRatio(size, resolution string) float64 {
	tier := imageSizeTier(size, resolution)
	switch tier {
	case "4k":
		return 4
	case "2k":
		return 2
	default:
		return 1
	}
}

func imageSizeTier(size, resolution string) string {
	text := strings.ToLower(strings.TrimSpace(firstNonEmpty(resolution, size)))
	if text == "" {
		return "1k"
	}
	if strings.Contains(text, "4k") {
		return "4k"
	}
	if strings.Contains(text, "2k") {
		return "2k"
	}
	if strings.Contains(text, "1k") {
		return "1k"
	}
	if w, h, ok := parsePixels(text); ok {
		maxSide := w
		if h > maxSide {
			maxSide = h
		}
		if maxSide >= 3000 {
			return "4k"
		}
		if maxSide >= 1500 {
			return "2k"
		}
	}
	return "1k"
}

var pixelSizeRe = regexp.MustCompile(`(?i)(\d{3,5})\s*x\s*(\d{3,5})`)

func parsePixels(s string) (int, int, bool) {
	matches := pixelSizeRe.FindStringSubmatch(s)
	if len(matches) != 3 {
		return 0, 0, false
	}
	w, errW := strconv.Atoi(matches[1])
	h, errH := strconv.Atoi(matches[2])
	return w, h, errW == nil && errH == nil
}

func ConvertStoredTask(task *model.Task) []byte {
	if len(task.Data) > 0 {
		var resp asyncGenerationResponse
		if err := common.Unmarshal(task.Data, &resp); err == nil {
			resp.ID = task.TaskID
			resp.TaskID = task.TaskID
			if resp.Object == "" {
				resp.Object = "async.generation"
			}
			if resp.Type == "" {
				resp.Type = "image"
			}
			applyTaskStatus(&resp, task)
			return normalizeResponseData(resp)
		}
	}
	return responseFromTaskStatus(task)
}

func IsCompletedTask(task *model.Task) bool {
	return task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
}

func responseFromTaskStatus(task *model.Task) []byte {
	resp := asyncGenerationResponse{
		ID:        task.TaskID,
		TaskID:    task.TaskID,
		Object:    "async.generation",
		Type:      "image",
		Model:     task.Properties.OriginModelName,
		CreatedAt: task.CreatedAt,
	}
	applyTaskStatus(&resp, task)
	return normalizeResponseData(resp)
}

func applyTaskStatus(resp *asyncGenerationResponse, task *model.Task) {
	switch task.Status {
	case model.TaskStatusSuccess:
		resp.Status = "completed"
		resp.Progress = 100
	case model.TaskStatusFailure:
		resp.Status = "failed"
		resp.Progress = 100
		if resp.Error == nil && task.FailReason != "" {
			resp.Error = &struct {
				Message string `json:"message,omitempty"`
				Code    string `json:"code,omitempty"`
			}{Message: task.FailReason, Code: "task_failed"}
		}
	case model.TaskStatusInProgress:
		resp.Status = "processing"
		if resp.Progress == 0 {
			resp.Progress = 30
		}
	default:
		resp.Status = "pending"
		if resp.Progress == 0 {
			resp.Progress = 0
		}
	}
	resp.Detail.Status = resp.Status
}
