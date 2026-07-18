package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType     int
	apiKey          string
	baseURL         string
	initialTaskInfo *relaycommon.TaskInfo
	asyncImageBody  []byte
	asyncImageInfo  asyncGeminiImageInfo
}

type asyncGeminiImageInfo struct {
	BaseURL      string
	APIKey       string
	Model        string
	PublicModel  string
	PublicTaskID string
	Proxy        string
}

const geminiImageResumeInProgressAfter = 10 * time.Minute

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// BuildRequestURL constructs the Gemini API predictLongRunning endpoint for Veo.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	modelName := info.UpstreamModelName
	version := model_setting.GetGeminiVersionSetting(modelName)
	if isGeminiImageTaskRequest(info) {
		return fmt.Sprintf(
			"%s/%s/models/%s:generateContent",
			a.baseURL,
			version,
			modelName,
		), nil
	}

	return fmt.Sprintf(
		"%s/%s/models/%s:predictLongRunning",
		a.baseURL,
		version,
		modelName,
	), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", a.apiKey)
	return nil
}

// BuildRequestBody converts request into the Veo predictLongRunning format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("unexpected task_request type")
	}
	if isGeminiImageTaskRequest(info) {
		body, err := buildGeminiImageRequestBody(req)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		a.asyncImageBody = data
		a.asyncImageInfo = asyncGeminiImageInfo{
			BaseURL:      a.baseURL,
			APIKey:       a.apiKey,
			Model:        info.UpstreamModelName,
			PublicModel:  info.OriginModelName,
			PublicTaskID: info.PublicTaskID,
			Proxy:        info.ChannelSetting.Proxy,
		}
		return bytes.NewReader(data), nil
	}

	instance := VeoInstance{Prompt: req.Prompt}
	if img := ExtractMultipartImage(c, info); img != nil {
		instance.Image = img
	} else if len(req.Images) > 0 {
		if parsed := ParseImageInput(req.Images[0]); parsed != nil {
			instance.Image = parsed
			info.Action = constant.TaskActionGenerate
		}
	}

	params := &VeoParameters{}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, params); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	if params.DurationSeconds == 0 && req.Duration > 0 {
		params.DurationSeconds = req.Duration
	}
	if params.Resolution == "" && req.Size != "" {
		params.Resolution = SizeToVeoResolution(req.Size)
	}
	if params.AspectRatio == "" && req.Size != "" {
		params.AspectRatio = SizeToVeoAspectRatio(req.Size)
	}
	params.Resolution = strings.ToLower(params.Resolution)
	params.SampleCount = 1

	body := VeoRequestPayload{
		Instances:  []VeoInstance{instance},
		Parameters: params,
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	if a.hasAsyncImageTask() || isGeminiImageTaskRequest(info) {
		if !a.hasAsyncImageTask() {
			data, err := io.ReadAll(requestBody)
			if err != nil {
				return nil, err
			}
			a.asyncImageBody = data
			a.asyncImageInfo = asyncGeminiImageInfo{
				BaseURL:      a.baseURL,
				APIKey:       a.apiKey,
				Model:        info.UpstreamModelName,
				PublicModel:  info.OriginModelName,
				PublicTaskID: info.PublicTaskID,
				Proxy:        info.ChannelSetting.Proxy,
			}
		}
		return newGeminiImageSubmittedHTTPResponse(info.PublicTaskID), nil
	}
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	if a.hasAsyncImageTask() || isGeminiImageTaskRequest(info) {
		return a.doImageSubmitResponse(c, responseBody, info)
	}

	var s submitResponse
	if err := common.Unmarshal(responseBody, &s); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(s.Name) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing operation name"), "invalid_response", http.StatusInternalServerError)
	}
	taskID = taskcommon.EncodeLocalTaskID(s.Name)
	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return taskID, responseBody, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"gemini-3-pro-image-preview",
		"veo-3.0-generate-001",
		"veo-3.0-fast-generate-001",
		"veo-3.1-generate-preview",
		"veo-3.1-fast-generate-preview",
	}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "gemini"
}

// EstimateBilling returns OtherRatios based on durationSeconds and resolution.
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	v, ok := c.Get("task_request")
	if !ok {
		return nil
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil
	}
	if isGeminiImageTaskRequest(info) {
		return map[string]float64{
			"price_tier": geminiImageTierPriceRatio(info.OriginModelName, info.UpstreamModelName, req),
			"n":          geminiImageCountRatio(req.N),
		}
	}

	seconds := ResolveVeoDuration(req.Metadata, req.Duration, req.Seconds)
	resolution := ResolveVeoResolution(req.Metadata, req.Size)
	resRatio := VeoResolutionRatio(info.UpstreamModelName, resolution)

	return map[string]float64{
		"seconds":    float64(seconds),
		"resolution": resRatio,
	}
}

func (a *TaskAdaptor) ValidateBilling(c *gin.Context, info *relaycommon.RelayInfo) error {
	if !isGeminiImageTaskRequest(info) {
		return nil
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return err
	}
	if _, ok := lookupGeminiConfiguredModelPrice(geminiImageModelPriceCandidates(info.OriginModelName, info.UpstreamModelName)...); !ok {
		return fmt.Errorf("model price is required for %s", info.OriginModelName)
	}
	tierKey := "@" + geminiImageTier(req.Size, req.Resolution)
	if _, ok := lookupGeminiConfiguredModelPrice(geminiImageTierPriceCandidates(tierKey, info.OriginModelName, info.UpstreamModelName)...); !ok {
		return fmt.Errorf("model price is required for %s", info.OriginModelName+tierKey)
	}
	return nil
}

func (a *TaskAdaptor) ResolveBillingModelName(info *relaycommon.RelayInfo) string {
	if info == nil || !isGeminiImageTaskRequest(info) {
		return ""
	}
	for _, name := range geminiImageModelPriceCandidates(info.OriginModelName, info.UpstreamModelName) {
		if _, ok := lookupGeminiConfiguredModelPrice(name); ok {
			return name
		}
	}
	return info.OriginModelName
}

func (a *TaskAdaptor) InitialTaskInfo() *relaycommon.TaskInfo {
	return a.initialTaskInfo
}

func (a *TaskAdaptor) hasAsyncImageTask() bool {
	return len(a.asyncImageBody) > 0 && strings.TrimSpace(a.asyncImageInfo.Model) != ""
}

func (a *TaskAdaptor) RunTaskAfterInsert(task *model.Task) {
	if task == nil || !a.hasAsyncImageTask() {
		return
	}
	body := append([]byte(nil), a.asyncImageBody...)
	info := a.asyncImageInfo
	if shouldPersistAsyncImageRequestBody(task) {
		task.PrivateData.RequestBody = base64.StdEncoding.EncodeToString(body)
		err := model.DB.Model(&model.Task{}).
			Where("id = ? AND status = ?", task.ID, task.Status).
			Update("private_data", task.PrivateData).Error
		if err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("persist gemini image request body failed for task %s: %s", task.TaskID, err.Error()))
		}
	}
	go runGeminiImageTask(context.Background(), task.ID, body, info)
}

func shouldPersistAsyncImageRequestBody(task *model.Task) bool {
	return task != nil &&
		!task.PrivateData.EphemeralInput &&
		strings.TrimSpace(task.PrivateData.RequestBody) == ""
}

type geminiImageTaskResponse struct {
	RequestID     string                 `json:"requestId,omitempty"`
	ModelCode     string                 `json:"modelCode,omitempty"`
	Status        string                 `json:"status,omitempty"`
	BillingStatus string                 `json:"billingStatus,omitempty"`
	Progress      int                    `json:"progress,omitempty"`
	CreateTime    string                 `json:"createTime,omitempty"`
	ResultCount   int                    `json:"resultCount,omitempty"`
	Result        map[string]interface{} `json:"result,omitempty"`
	URL           string                 `json:"url,omitempty"`
	Error         *struct {
		Message string `json:"message,omitempty"`
		Code    string `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

func newGeminiImageSubmittedHTTPResponse(requestID string) *http.Response {
	body, _ := common.Marshal(geminiImageTaskResponse{
		RequestID: requestID,
		Status:    "submitted",
		Progress:  20,
	})
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}

func (a *TaskAdaptor) doImageSubmitResponse(c *gin.Context, responseBody []byte, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	var upstream geminiImageTaskResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(upstream.RequestID) == "" {
		upstream.RequestID = info.PublicTaskID
	}
	upstream.ModelCode = info.OriginModelName
	upstream.BillingStatus = "pending"
	upstream.Status = "submitted"
	upstream.Progress = 20
	if upstream.CreateTime == "" {
		upstream.CreateTime = time.Now().Format("2006-01-02 15:04:05")
	}
	data, err := common.Marshal(upstream)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	a.initialTaskInfo = &relaycommon.TaskInfo{
		TaskID:   info.PublicTaskID,
		Status:   model.TaskStatusSubmitted,
		Progress: taskcommon.ProgressSubmitted,
		Data:     data,
	}
	c.JSON(http.StatusOK, upstream)
	return info.PublicTaskID, data, nil
}

func IsImageTaskModel(modelName string) bool {
	return isGeminiImageTaskModel(modelName)
}

func isGeminiImageTaskRequest(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	return info.RelayMode == relayconstant.RelayModeAsyncImageSubmit ||
		isGeminiImageTaskModel(info.UpstreamModelName)
}

func TryResumeImageTask(task *model.Task, baseURL, apiKey, proxy string) bool {
	if task == nil {
		return false
	}
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		return false
	}
	if task.Status == model.TaskStatusInProgress {
		if task.StartTime > 0 && time.Since(time.Unix(task.StartTime, 0)) < geminiImageResumeInProgressAfter {
			return false
		}
	}
	requestBody := strings.TrimSpace(task.PrivateData.RequestBody)
	if requestBody == "" {
		return false
	}
	body, err := base64.StdEncoding.DecodeString(requestBody)
	if err != nil {
		logger.LogError(context.Background(), fmt.Sprintf("decode gemini image request body failed for task %s: %s", task.TaskID, err.Error()))
		return false
	}
	info := asyncGeminiImageInfo{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		Model:        task.Properties.UpstreamModelName,
		PublicModel:  publicGeminiImageTaskModel(task),
		PublicTaskID: task.TaskID,
		Proxy:        proxy,
	}
	go runGeminiImageTask(context.Background(), task.ID, body, info)
	return true
}

func runGeminiImageTask(ctx context.Context, taskID int64, requestBody []byte, info asyncGeminiImageInfo) {
	task := &model.Task{}
	if err := model.DB.First(task, taskID).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("get gemini image task failed: %s", err.Error()))
		return
	}
	progressDone := make(chan struct{})
	defer close(progressDone)
	updateTask := func(taskInfo *relaycommon.TaskInfo, fallback []byte) {
		if taskInfo != nil && taskInfo.Status == model.TaskStatusFailure {
			resubmitted, resubmitErr := service.TryResubmitAsyncImageTask(ctx, task, taskInfo.Reason)
			if resubmitErr != nil {
				logger.LogWarn(ctx, fmt.Sprintf("resubmit gemini image task %s failed: %s", task.TaskID, resubmitErr.Error()))
			}
			if resubmitted {
				return
			}
		}
		snap := task.Snapshot()
		applyGeminiImageTaskInfo(task, taskInfo, fallback)
		isDone := task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure
		clearPrivateData := isDone && task.HasTransientPrivateData()
		if clearPrivateData {
			task.ClearTransientPrivateData()
		}
		if !snap.Equal(task.Snapshot()) || clearPrivateData {
			won, updateErr := task.UpdateWithSnapshot(snap, clearPrivateData)
			if updateErr != nil {
				logger.LogError(ctx, fmt.Sprintf("update gemini image task %s failed: %s", task.TaskID, updateErr.Error()))
				return
			}
			if won && snap.Status != task.Status {
				switch task.Status {
				case model.TaskStatusFailure:
					if task.Quota != 0 {
						service.RefundTaskQuota(ctx, task, task.FailReason)
					}
				case model.TaskStatusSuccess:
					service.SettleTaskBillingOnComplete(ctx, &TaskAdaptor{}, task, taskInfo)
				}
			}
		}
	}

	updateTask(&relaycommon.TaskInfo{Status: model.TaskStatusInProgress, Progress: taskcommon.ProgressInProgress}, nil)
	go simulateGeminiImageProgress(ctx, task.ID, progressDone)
	respBody, err := callGeminiImageGenerate(ctx, requestBody, info)
	if err != nil {
		updateTask(failedGeminiImageTaskInfo(task.TaskID, info.PublicModel, err), nil)
		return
	}
	taskInfo, data, err := parseGeminiImageCompletion(task.TaskID, info.PublicModel, respBody)
	if err != nil {
		updateTask(failedGeminiImageTaskInfo(task.TaskID, info.PublicModel, err), nil)
		return
	}
	updateTask(taskInfo, data)
}

func simulateGeminiImageProgress(ctx context.Context, taskID int64, done <-chan struct{}) {
	progresses := []string{"45%", "60%", "75%", "88%", "95%"}
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for _, progress := range progresses {
		select {
		case <-done:
			return
		case <-ticker.C:
			task := &model.Task{}
			if err := model.DB.First(task, taskID).Error; err != nil {
				logger.LogError(ctx, fmt.Sprintf("get gemini image task for progress failed: %s", err.Error()))
				return
			}
			if task.Status != model.TaskStatusInProgress {
				return
			}
			snap := task.Snapshot()
			task.Progress = progress
			won, err := task.UpdateWithSnapshot(snap, false)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("update gemini image task progress %s failed: %s", task.TaskID, err.Error()))
				return
			}
			if !won {
				return
			}
		}
	}
}

func callGeminiImageGenerate(ctx context.Context, requestBody []byte, info asyncGeminiImageInfo) ([]byte, error) {
	version := model_setting.GetGeminiVersionSetting(info.Model)
	url := fmt.Sprintf("%s/%s/models/%s:generateContent", info.BaseURL, version, info.Model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", info.APIKey)
	client, err := service.GetHttpClientWithProxy(info.Proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("gemini image status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func parseGeminiImageCompletion(requestID, modelCode string, responseBody []byte) (*relaycommon.TaskInfo, []byte, error) {
	var geminiResp dto.GeminiChatResponse
	if err := common.Unmarshal(responseBody, &geminiResp); err != nil {
		return nil, nil, errors.Wrapf(err, "body: %s", responseBody)
	}
	saved, err := collectGeminiImageOutputs(geminiResp)
	if err != nil {
		return nil, nil, err
	}
	publicResp := geminiImagePublicResponse(requestID, modelCode, saved)
	data, err := common.Marshal(publicResp)
	if err != nil {
		return nil, nil, err
	}
	return &relaycommon.TaskInfo{
		TaskID:   requestID,
		Status:   model.TaskStatusSuccess,
		Url:      publicResp.URL,
		Progress: taskcommon.ProgressComplete,
		Data:     data,
	}, data, nil
}

func failedGeminiImageTaskInfo(requestID, modelCode string, err error) *relaycommon.TaskInfo {
	resp := geminiImageTaskResponse{
		RequestID: requestID,
		ModelCode: modelCode,
		Status:    "failed",
		Progress:  100,
		Error: &struct {
			Message string `json:"message,omitempty"`
			Code    string `json:"code,omitempty"`
		}{Message: err.Error(), Code: "task_failed"},
	}
	data, _ := common.Marshal(resp)
	return &relaycommon.TaskInfo{
		TaskID:   requestID,
		Status:   model.TaskStatusFailure,
		Reason:   err.Error(),
		Progress: taskcommon.ProgressComplete,
		Data:     data,
	}
}

func applyGeminiImageTaskInfo(task *model.Task, taskInfo *relaycommon.TaskInfo, fallback []byte) {
	if taskInfo == nil {
		return
	}
	now := time.Now().Unix()
	if taskInfo.Status != "" {
		task.Status = model.TaskStatus(taskInfo.Status)
	}
	switch task.Status {
	case model.TaskStatusSuccess:
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.PrivateData.ResultURL = taskInfo.Url
	case model.TaskStatusFailure:
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskInfo.Reason
	case model.TaskStatusInProgress:
		task.Progress = taskcommon.ProgressInProgress
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		task.Progress = taskcommon.ProgressQueued
	}
	if taskInfo.Progress != "" {
		task.Progress = taskInfo.Progress
	}
	if len(taskInfo.Data) > 0 {
		task.Data = taskInfo.Data
	} else if len(fallback) > 0 {
		task.Data = fallback
	}
}

func isGeminiImageTaskModel(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	modelName = strings.TrimPrefix(modelName, "models/")
	if idx := strings.Index(modelName, ":"); idx >= 0 {
		modelName = modelName[:idx]
	}
	if strings.Contains(modelName, "image") && strings.HasPrefix(modelName, "gemini-") {
		return true
	}
	return strings.HasPrefix(modelName, "nano-banana")
}

func buildGeminiImageRequestBody(req relaycommon.TaskSubmitReq) (io.Reader, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("prompt field is required")
	}
	parts := make([]dto.GeminiPart, 0, len(geminiImageInputs(req))+1)
	for _, image := range geminiImageInputs(req) {
		part, err := geminiImageInputPart(image)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	parts = append(parts, dto.GeminiPart{Text: req.Prompt})

	imageConfig, err := geminiImageConfig(req)
	if err != nil {
		return nil, err
	}
	geminiReq := dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{
				Role:  "user",
				Parts: parts,
			},
		},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ResponseModalities: []string{"IMAGE"},
			ImageConfig:        imageConfig,
		},
	}
	if req.N > 0 {
		geminiReq.GenerationConfig.CandidateCount = &req.N
	}
	data, err := common.Marshal(geminiReq)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func geminiImageInputs(req relaycommon.TaskSubmitReq) []string {
	images := make([]string, 0, 1+len(req.Images)+len(req.ReferenceImages)+len(req.ReferenceImageURLs))
	add := func(image string) {
		image = strings.TrimSpace(image)
		if image != "" {
			images = append(images, image)
		}
	}
	add(req.Image)
	for _, image := range req.Images {
		add(image)
	}
	for _, image := range req.ReferenceImages {
		add(image)
	}
	for _, image := range req.ReferenceImageURLs {
		add(image)
	}
	return uniqueGeminiStrings(images)
}

func geminiImageInputPart(image string) (dto.GeminiPart, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return dto.GeminiPart{}, fmt.Errorf("image input is empty")
	}
	if isHTTPURL(image) {
		return dto.GeminiPart{
			FileData: &dto.GeminiFileData{
				MimeType: inferImageMimeTypeFromURL(image),
				FileUri:  image,
			},
		}, nil
	}
	parsed := ParseImageInput(image)
	if parsed == nil {
		return dto.GeminiPart{}, fmt.Errorf("invalid image input")
	}
	return dto.GeminiPart{
		InlineData: &dto.GeminiInlineData{
			MimeType: parsed.MimeType,
			Data:     parsed.BytesBase64Encoded,
		},
	}, nil
}

func geminiImageConfig(req relaycommon.TaskSubmitReq) ([]byte, error) {
	config := map[string]string{}
	if aspect := geminiImageAspect(req.Size, req.AspectRatio); aspect != "" {
		config["aspectRatio"] = aspect
	}
	if size := strings.ToUpper(geminiImageExplicitTier(req.Size, req.Resolution)); size != "" {
		config["imageSize"] = size
	}
	if len(config) == 0 {
		return nil, nil
	}
	return common.Marshal(config)
}

func (a *TaskAdaptor) doImageResponse(c *gin.Context, responseBody []byte, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	var geminiResp dto.GeminiChatResponse
	if err := common.Unmarshal(responseBody, &geminiResp); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_failed", http.StatusInternalServerError)
	}
	saved, err := collectGeminiImageOutputs(geminiResp)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "save_result_file_failed", http.StatusInternalServerError)
	}
	publicResp := geminiImagePublicResponse(info.PublicTaskID, info.OriginModelName, saved)
	taskData, err := common.Marshal(publicResp)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	a.initialTaskInfo = &relaycommon.TaskInfo{
		TaskID:   info.PublicTaskID,
		Status:   model.TaskStatusSuccess,
		Url:      publicResp.URL,
		Progress: "100%",
		Data:     taskData,
	}
	c.JSON(http.StatusOK, publicResp)
	return info.PublicTaskID, taskData, nil
}

var geminiMarkdownImageURLRe = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^)\s]+)\)`)

func collectGeminiImageOutputs(resp dto.GeminiChatResponse) ([]string, error) {
	saved := make([]string, 0)
	savedRemoteURLs := make(map[string]string)
	saveRemoteURL := func(rawURL string) error {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			return nil
		}
		if ossURL, ok := savedRemoteURLs[rawURL]; ok {
			saved = appendUniqueGeminiImageOutput(saved, ossURL)
			return nil
		}
		ossURL, err := saveGeminiImageURL(rawURL)
		if err != nil {
			return err
		}
		savedRemoteURLs[rawURL] = ossURL
		saved = appendUniqueGeminiImageOutput(saved, ossURL)
		return nil
	}
	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.FileData != nil && isGeminiImageFileData(part.FileData) {
				if err := saveRemoteURL(part.FileData.FileUri); err != nil {
					return nil, err
				}
			}
			for _, url := range geminiMarkdownImageURLs(part.Text) {
				if err := saveRemoteURL(url); err != nil {
					return nil, err
				}
			}
			if part.InlineData == nil || !strings.HasPrefix(strings.ToLower(part.InlineData.MimeType), "image/") {
				continue
			}
			ossURL, err := service.SaveBase64ImageToAliyunOSS(part.InlineData.Data, part.InlineData.MimeType)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(ossURL) == "" {
				if len(saved) == 0 {
					return nil, fmt.Errorf("aliyun oss is not enabled or configured")
				}
				continue
			}
			saved = appendUniqueGeminiImageOutput(saved, ossURL)
		}
	}
	if len(saved) == 0 {
		return nil, fmt.Errorf("completed response contains no result image")
	}
	return saved, nil
}

func saveGeminiImageURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil
	}
	return service.StrictSaveImageURLToAliyunOSS(rawURL, "")
}

func isGeminiImageFileData(fileData *dto.GeminiFileData) bool {
	if fileData == nil || strings.TrimSpace(fileData.FileUri) == "" {
		return false
	}
	mimeType := strings.ToLower(strings.TrimSpace(fileData.MimeType))
	return mimeType == "" || strings.HasPrefix(mimeType, "image/")
}

func geminiMarkdownImageURLs(text string) []string {
	matches := geminiMarkdownImageURLRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	urls := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			urls = append(urls, match[1])
		}
	}
	return urls
}

func appendUniqueGeminiImageOutput(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func geminiImagePublicResponse(requestID, modelCode string, saved []string) geminiImageTaskResponse {
	items := make([]interface{}, 0, len(saved))
	for _, url := range saved {
		items = append(items, map[string]interface{}{
			"url":  url,
			"type": "image",
		})
	}
	result := map[string]interface{}{
		"image_url": saved[0],
		"items":     items,
		"url":       saved[0],
	}
	if len(saved) > 1 {
		result["image_urls"] = saved
	}
	return geminiImageTaskResponse{
		RequestID:   requestID,
		ModelCode:   modelCode,
		Status:      "succeeded",
		Progress:    100,
		ResultCount: len(saved),
		Result:      result,
		URL:         saved[0],
	}
}

func ConvertStoredImageTask(task *model.Task) []byte {
	if len(task.Data) > 0 {
		var resp geminiImageTaskResponse
		if err := common.Unmarshal(task.Data, &resp); err == nil {
			resp.RequestID = task.TaskID
			resp.ModelCode = publicGeminiImageTaskModel(task)
			applyGeminiImageTaskStatus(&resp, task)
			data, err := common.Marshal(resp)
			if err == nil {
				return data
			}
		}
	}
	resp := geminiImageTaskResponse{
		RequestID: task.TaskID,
		ModelCode: publicGeminiImageTaskModel(task),
	}
	applyGeminiImageTaskStatus(&resp, task)
	data, _ := common.Marshal(resp)
	return data
}

func publicGeminiImageTaskModel(task *model.Task) string {
	if task == nil {
		return ""
	}
	if task.Properties.OriginModelName != "" {
		return task.Properties.OriginModelName
	}
	return task.Properties.UpstreamModelName
}

func applyGeminiImageTaskStatus(resp *geminiImageTaskResponse, task *model.Task) {
	switch task.Status {
	case model.TaskStatusSuccess:
		resp.Status = "succeeded"
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
}

func geminiImageTier(size, resolution string) string {
	text := strings.ToLower(strings.TrimSpace(firstNonEmpty(resolution, size)))
	switch {
	case strings.Contains(text, "4k"):
		return "4k"
	case strings.Contains(text, "2k"):
		return "2k"
	}
	if tier := geminiImagePixelTier(text); tier != "" {
		return tier
	}
	return "1k"
}

func geminiImageExplicitTier(size, resolution string) string {
	text := strings.ToLower(strings.TrimSpace(resolution))
	if text == "" {
		text = strings.ToLower(strings.TrimSpace(size))
		if !strings.Contains(text, "1k") && !strings.Contains(text, "2k") && !strings.Contains(text, "4k") {
			return ""
		}
	}
	switch {
	case strings.Contains(text, "4k"):
		return "4k"
	case strings.Contains(text, "2k"):
		return "2k"
	case strings.Contains(text, "1k"):
		return "1k"
	}
	return geminiImagePixelTier(text)
}

func geminiImagePixelTier(text string) string {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(text)), "x", 2)
	if len(parts) != 2 {
		return ""
	}
	w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	if w <= 32 || h <= 32 {
		return ""
	}
	area := w * h
	switch {
	case area >= 8_000_000:
		return "4k"
	case area >= 2_000_000:
		return "2k"
	default:
		return "1k"
	}
}

func geminiImageAspect(size, aspectRatio string) string {
	if aspect := normalizeGeminiAspectRatio(aspectRatio); aspect != "" {
		return aspect
	}
	return normalizeGeminiAspectRatio(size)
}

func normalizeGeminiAspectRatio(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" || text == "auto" {
		return ""
	}
	if idx := strings.Index(text, "-"); idx >= 0 {
		text = text[:idx]
	}
	text = strings.ReplaceAll(text, "_", ":")
	if strings.Contains(text, "x") {
		parts := strings.SplitN(text, "x", 2)
		if len(parts) == 2 {
			w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
			h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			if w > 0 && h > 0 {
				if w <= 32 && h <= 32 {
					return supportedGeminiAspect(fmt.Sprintf("%d:%d", w, h))
				}
				return nearestGeminiAspect(w, h)
			}
		}
	}
	if strings.Contains(text, ":") {
		parts := strings.SplitN(text, ":", 2)
		w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		if w > 0 && h > 0 {
			return supportedGeminiAspect(fmt.Sprintf("%d:%d", w, h))
		}
	}
	return ""
}

func supportedGeminiAspect(aspect string) string {
	switch aspect {
	case "1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9":
		return aspect
	default:
		return ""
	}
}

func nearestGeminiAspect(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	ratio := float64(w) / float64(h)
	bestAspect := ""
	bestDiff := 10.0
	for _, item := range []struct {
		aspect string
		ratio  float64
	}{
		{"1:1", 1},
		{"2:3", 2.0 / 3.0},
		{"3:2", 3.0 / 2.0},
		{"3:4", 3.0 / 4.0},
		{"4:3", 4.0 / 3.0},
		{"4:5", 4.0 / 5.0},
		{"5:4", 5.0 / 4.0},
		{"9:16", 9.0 / 16.0},
		{"16:9", 16.0 / 9.0},
		{"21:9", 21.0 / 9.0},
	} {
		diff := ratio - item.ratio
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			bestAspect = item.aspect
		}
	}
	return bestAspect
}

func geminiImageTierPriceRatio(originModel, upstreamModel string, req relaycommon.TaskSubmitReq) float64 {
	basePrice, ok := lookupGeminiConfiguredModelPrice(geminiImageModelPriceCandidates(originModel, upstreamModel)...)
	if !ok || basePrice <= 0 {
		return 1
	}
	tierPrice, ok := lookupGeminiConfiguredModelPrice(geminiImageTierPriceCandidates("@"+geminiImageTier(req.Size, req.Resolution), originModel, upstreamModel)...)
	if !ok || tierPrice <= 0 {
		return 1
	}
	return tierPrice / basePrice
}

func geminiImageCountRatio(n int) float64 {
	if n <= 0 {
		return 1
	}
	return float64(n)
}

func geminiImageModelPriceCandidates(originModel, upstreamModel string) []string {
	names := make([]string, 0, 2)
	for _, name := range []string{originModel, upstreamModel} {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return uniqueGeminiStrings(names)
}

func geminiImageTierPriceCandidates(tierKey, originModel, upstreamModel string) []string {
	baseNames := geminiImageModelPriceCandidates(originModel, upstreamModel)
	names := make([]string, 0, len(baseNames))
	for _, name := range baseNames {
		names = append(names, name+tierKey)
	}
	return uniqueGeminiStrings(names)
}

func lookupGeminiConfiguredModelPrice(names ...string) (float64, bool) {
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if price, ok := ratio_setting.GetModelPrice(name, false); ok {
			return price, true
		}
	}
	return 0, false
}

func isHTTPURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func inferImageMimeTypeFromURL(rawURL string) string {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	if idx := strings.Index(lower, "?"); idx >= 0 {
		lower = lower[:idx]
	}
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uniqueGeminiStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// FetchTask polls task status via the Gemini operations GET endpoint.
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	upstreamName, err := taskcommon.DecodeLocalTaskID(taskID)
	if err != nil {
		return nil, fmt.Errorf("decode task_id failed: %w", err)
	}

	version := model_setting.GetGeminiVersionSetting("default")
	url := fmt.Sprintf("%s/%s/%s", baseUrl, version, upstreamName)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var op operationResponse
	if err := common.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal operation response failed: %w", err)
	}

	ti := &relaycommon.TaskInfo{}

	if op.Error.Message != "" {
		ti.Status = model.TaskStatusFailure
		ti.Reason = op.Error.Message
		ti.Progress = "100%"
		return ti, nil
	}

	if !op.Done {
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "50%"
		return ti, nil
	}

	ti.Status = model.TaskStatusSuccess
	ti.Progress = "100%"

	ti.TaskID = taskcommon.EncodeLocalTaskID(op.Name)

	if len(op.Response.GenerateVideoResponse.GeneratedVideos) > 0 {
		if uri := op.Response.GenerateVideoResponse.GeneratedVideos[0].Video.URI; uri != "" {
			ti.RemoteUrl = uri
		}
	}

	return ti, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	upstreamTaskID := task.GetUpstreamTaskID()
	upstreamName, err := taskcommon.DecodeLocalTaskID(upstreamTaskID)
	if err != nil {
		upstreamName = ""
	}
	modelName := extractModelFromOperationName(upstreamName)
	if strings.TrimSpace(modelName) == "" {
		modelName = "veo-3.0-generate-001"
	}

	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.Model = modelName
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt = task.CreatedAt
	if task.FinishTime > 0 {
		video.CompletedAt = task.FinishTime
	} else if task.UpdatedAt > 0 {
		video.CompletedAt = task.UpdatedAt
	}

	return common.Marshal(video)
}

// ============================
// helpers
// ============================

var modelRe = regexp.MustCompile(`models/([^/]+)/operations/`)

func extractModelFromOperationName(name string) string {
	if name == "" {
		return ""
	}
	if m := modelRe.FindStringSubmatch(name); len(m) == 2 {
		return m[1]
	}
	if idx := strings.Index(name, "models/"); idx >= 0 {
		s := name[idx+len("models/"):]
		if p := strings.Index(s, "/operations/"); p > 0 {
			return s[:p]
		}
	}
	return ""
}
