package relay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func init() {
	service.TryResubmitAsyncImageTaskFunc = TryResubmitAsyncImageTask
}

func TryResubmitAsyncImageTask(ctx context.Context, task *model.Task, reason string) (bool, error) {
	if !isAsyncImageFailoverTask(task) {
		return false, nil
	}

	current := &model.Task{}
	if err := model.DB.First(current, task.ID).Error; err != nil {
		return false, err
	}
	if current.Status == model.TaskStatusSuccess || current.Status == model.TaskStatusFailure {
		return false, nil
	}

	req, err := taskOriginalRequest(current)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = firstNonEmptyString(current.Properties.OriginModelName, current.Properties.UpstreamModelName)
	}
	modelName := firstNonEmptyString(current.Properties.OriginModelName, req.Model, current.Properties.UpstreamModelName)
	if strings.TrimSpace(modelName) == "" {
		return false, nil
	}

	candidates, err := model.GetEnabledChannelsForGroupModel(current.Group, modelName)
	if err != nil {
		return false, err
	}
	if len(candidates) == 0 {
		return false, nil
	}

	tried := taskTriedChannelSet(current)
	if current.ChannelId > 0 {
		tried[current.ChannelId] = struct{}{}
	}

	lastErr := strings.TrimSpace(reason)
	for _, candidate := range candidates {
		if _, ok := tried[candidate.Id]; ok {
			continue
		}
		if !supportsImageFailoverChannel(candidate, modelName) {
			continue
		}
		tried[candidate.Id] = struct{}{}

		result, info, submitErr := resubmitImageToChannel(ctx, current, req, modelName, candidate)
		if submitErr != nil {
			lastErr = fmt.Sprintf("channel #%d submit failed: %s", candidate.Id, submitErr.Error())
			logger.LogWarn(ctx, fmt.Sprintf("async image task %s failover candidate #%d failed: %s", current.TaskID, candidate.Id, submitErr.Error()))
			continue
		}

		fromStatus := current.Status
		applyAsyncImageResubmitResult(current, result, info, candidate, tried, reason)
		won, updateErr := current.UpdateWithStatus(fromStatus)
		if updateErr != nil {
			return false, updateErr
		}
		if !won {
			return true, nil
		}

		RunTaskAfterInsert(result, current)
		*task = *current
		logger.LogInfo(ctx, fmt.Sprintf("async image task %s resubmitted to channel #%d", current.TaskID, candidate.Id))
		return true, nil
	}

	current.PrivateData.TriedChannelIDs = taskTriedChannelList(tried)
	current.PrivateData.LastFailureReason = lastErr
	task.PrivateData.TriedChannelIDs = current.PrivateData.TriedChannelIDs
	task.PrivateData.LastFailureReason = current.PrivateData.LastFailureReason
	if err := persistAsyncImageFailoverPrivateData(current); err != nil {
		return false, err
	}
	return false, nil
}

func isAsyncImageFailoverTask(task *model.Task) bool {
	if task == nil {
		return false
	}
	if strings.TrimSpace(task.PrivateData.OriginalRequest) == "" {
		return false
	}
	if channelType, err := strconv.Atoi(string(task.Platform)); err == nil && supportsAsyncImageFailoverChannel(channelType) {
		return true
	}
	modelName := strings.ToLower(firstNonEmptyString(task.Properties.OriginModelName, task.Properties.UpstreamModelName))
	return strings.Contains(modelName, "image") || task.PrivateData.ImageProtocol != ""
}

func taskOriginalRequest(task *model.Task) (relaycommon.TaskSubmitReq, error) {
	var req relaycommon.TaskSubmitReq
	raw := strings.TrimSpace(task.PrivateData.OriginalRequest)
	if raw == "" {
		return req, fmt.Errorf("original request is empty")
	}
	if err := common.Unmarshal([]byte(raw), &req); err != nil {
		return req, fmt.Errorf("unmarshal original request failed: %w", err)
	}
	return req, nil
}

func supportsAsyncImageFailoverChannel(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeMihuifang, constant.ChannelTypeFirefly, constant.ChannelTypeGemini:
		return true
	default:
		return false
	}
}

func supportsImageFailoverChannel(ch *model.Channel, modelName string) bool {
	if ch == nil {
		return false
	}
	upstreamModel := resolveChannelMappedModelName(ch, modelName)
	return supportsAsyncImageFailoverChannel(ch.Type) ||
		supportsSyncImageFailoverChannel(ch.Type, upstreamModel)
}

func supportsSyncImageFailoverChannel(channelType int, modelName string) bool {
	// Firefly exposes image generation through the task adaptor's
	// /v1/chat/completions bridge only. Falling back to the generic OpenAI image
	// adaptor would issue an unsupported /v1/images/generations request and can
	// replace the original, sanitized error with the raw fallback response.
	if channelType == constant.ChannelTypeFirefly {
		return false
	}
	if _, ok := common.ChannelType2APIType(channelType); !ok {
		return false
	}
	if channelType == constant.ChannelTypeGemini {
		return supportsGeminiSyncImageModel(modelName)
	}
	return endpointTypesContain(common.GetEndpointTypesByChannelType(channelType, modelName), constant.EndpointTypeImageGeneration) ||
		endpointTypesContain(model.GetModelSupportEndpointTypes(modelName), constant.EndpointTypeImageGeneration)
}

func endpointTypesContain(endpointTypes []constant.EndpointType, target constant.EndpointType) bool {
	for _, endpointType := range endpointTypes {
		if endpointType == target {
			return true
		}
	}
	return false
}

func resubmitImageToChannel(ctx context.Context, task *model.Task, req relaycommon.TaskSubmitReq, modelName string, ch *model.Channel) (*TaskSubmitResult, *relaycommon.RelayInfo, error) {
	var lastErr error
	upstreamModel := resolveChannelMappedModelName(ch, modelName)
	if supportsAsyncImageFailoverChannel(ch.Type) {
		result, info, err := resubmitAsyncImageToChannel(ctx, task, req, modelName, ch)
		if err == nil {
			return result, info, nil
		}
		lastErr = err
	}
	if supportsSyncImageFailoverChannel(ch.Type, upstreamModel) {
		result, info, err := resubmitSyncImageToChannel(ctx, task, req, modelName, ch)
		if err == nil {
			return result, info, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, fmt.Errorf("channel does not support image failover")
}

func resubmitAsyncImageToChannel(ctx context.Context, task *model.Task, req relaycommon.TaskSubmitReq, modelName string, ch *model.Channel) (*TaskSubmitResult, *relaycommon.RelayInfo, error) {
	c, err := newAsyncImageFailoverContext(ctx, task, req, modelName)
	if err != nil {
		return nil, nil, err
	}
	if apiErr := middleware.SetupContextForSelectedChannel(c, ch, modelName); apiErr != nil {
		return nil, nil, apiErr.Err
	}

	info := &relaycommon.RelayInfo{
		TokenGroup:      task.Group,
		UserId:          task.UserId,
		UsingGroup:      task.Group,
		UserGroup:       task.Group,
		StartTime:       time.Now(),
		RelayMode:       relayconstant.RelayModeAsyncImageSubmit,
		OriginModelName: modelName,
		RequestURLPath:  "/v1/async/generations",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: task.TaskID,
		},
	}
	info.InitChannelMeta(c)
	info.UpstreamModelName = modelName

	platform := constant.TaskPlatform(strconv.Itoa(ch.Type))
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return nil, nil, fmt.Errorf("task adaptor not found for channel type %d", ch.Type)
	}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return nil, nil, taskErrorToError(taskErr)
	}

	info.OriginModelName = modelName
	info.UpstreamModelName = modelName
	if err = helper.ModelMappedHelper(c, info, nil); err != nil {
		return nil, nil, err
	}
	if validator, ok := adaptor.(interface {
		ValidateBilling(c *gin.Context, info *relaycommon.RelayInfo) error
	}); ok {
		if err = validator.ValidateBilling(c, info); err != nil {
			return nil, nil, err
		}
	}
	if err = calculateTaskSubmissionPrice(c, info, adaptor, modelName); err != nil {
		return nil, nil, err
	}

	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, nil, err
	}
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, nil, err
	}
	if resp != nil && !isHTTPSuccessStatus(resp.StatusCode) {
		responseBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		return nil, nil, taskErrorToError(taskErr)
	}

	var initialTaskInfo *relaycommon.TaskInfo
	if provider, ok := adaptor.(taskInitialInfoProvider); ok {
		initialTaskInfo = provider.InitialTaskInfo()
	}
	var postInsertRunner taskPostInsertRunner
	if runner, ok := adaptor.(taskPostInsertRunner); ok {
		postInsertRunner = runner
	}

	return &TaskSubmitResult{
		UpstreamTaskID:   upstreamTaskID,
		TaskData:         taskData,
		Platform:         platform,
		Quota:            info.PriceData.Quota,
		ImageProtocol:    relayImageProtocol(info),
		InitialTaskInfo:  initialTaskInfo,
		postInsertRunner: postInsertRunner,
	}, info, nil
}

func resubmitSyncImageToChannel(ctx context.Context, task *model.Task, req relaycommon.TaskSubmitReq, modelName string, ch *model.Channel) (*TaskSubmitResult, *relaycommon.RelayInfo, error) {
	imageReq, err := taskSubmitReqToImageRequest(req, modelName)
	if err != nil {
		return nil, nil, err
	}
	c, recorder, err := newSyncImageFailoverContext(task, imageReq, modelName)
	if err != nil {
		return nil, nil, err
	}
	if apiErr := middleware.SetupContextForSelectedChannel(c, ch, modelName); apiErr != nil {
		return nil, nil, apiErr.Err
	}

	action := constant.TaskActionTextGenerate
	if req.HasImage() {
		action = constant.TaskActionGenerate
	}
	info := &relaycommon.RelayInfo{
		TokenGroup:      task.Group,
		UserId:          task.UserId,
		UsingGroup:      task.Group,
		UserGroup:       task.Group,
		StartTime:       time.Now(),
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: modelName,
		RequestURLPath:  "/v1/images/generations",
		Request:         &imageReq,
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: task.TaskID,
			Action:       action,
		},
	}
	info.InitChannelMeta(c)
	info.UpstreamModelName = modelName

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, nil, fmt.Errorf("image adaptor not found for api type %d", info.ApiType)
	}
	adaptor.Init(info)
	if err = helper.ModelMappedHelper(c, info, &imageReq); err != nil {
		return nil, nil, err
	}
	requestBody, err := buildSyncImageFailoverRequestBody(c, info, adaptor, imageReq)
	if err != nil {
		return nil, nil, err
	}
	respAny, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, nil, err
	}
	httpResp, ok := respAny.(*http.Response)
	if !ok || httpResp == nil {
		return nil, nil, fmt.Errorf("image adaptor returned invalid response type %T", respAny)
	}
	if !isHTTPSuccessStatus(httpResp.StatusCode) {
		responseBody, _ := io.ReadAll(httpResp.Body)
		_ = httpResp.Body.Close()
		return nil, nil, fmt.Errorf("status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	usage, apiErr := adaptor.DoResponse(c, httpResp, info)
	if apiErr != nil {
		return nil, nil, apiErr
	}
	taskInfo, taskData, err := syncImageResponseToTaskInfo(task.TaskID, info.OriginModelName, recorder.Body.Bytes())
	if err != nil {
		return nil, nil, err
	}
	if taskInfo.TotalTokens == 0 {
		if imageUsage, ok := usage.(*dto.Usage); ok && imageUsage != nil {
			taskInfo.TotalTokens = imageUsage.TotalTokens
		}
	}

	return &TaskSubmitResult{
		UpstreamTaskID:  task.TaskID,
		TaskData:        taskData,
		Platform:        constant.TaskPlatform(strconv.Itoa(ch.Type)),
		Quota:           task.Quota,
		ImageProtocol:   "sync-image",
		InitialTaskInfo: taskInfo,
	}, info, nil
}

func buildSyncImageFailoverRequestBody(c *gin.Context, info *relaycommon.RelayInfo, adaptor interface {
	ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error)
}, imageReq dto.ImageRequest) (io.Reader, error) {
	convertedRequest, err := adaptor.ConvertImageRequest(c, info, imageReq)
	if err != nil {
		return nil, err
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
	if requestBody, ok := convertedRequest.(io.Reader); ok {
		return requestBody, nil
	}

	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, err
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return nil, err
		}
	}
	info.UpstreamRequestBodySize = int64(len(jsonData))
	return bytes.NewReader(jsonData), nil
}

func newAsyncImageFailoverContext(ctx context.Context, task *model.Task, req relaycommon.TaskSubmitReq, modelName string) (*gin.Context, error) {
	body, err := common.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/async/generations", bytes.NewReader(body))
	if ctx != nil {
		httpReq = httpReq.WithContext(ctx)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httpReq
	c.Set("id", task.UserId)
	c.Set("relay_mode", relayconstant.RelayModeAsyncImageSubmit)
	relaycommon.SetTaskRequestForFailover(c, req)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, task.Group)
	common.SetContextKey(c, constant.ContextKeyUserGroup, task.Group)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	return c, nil
}

func newSyncImageFailoverContext(task *model.Task, req dto.ImageRequest, modelName string) (*gin.Context, *httptest.ResponseRecorder, error) {
	body, err := common.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httpReq
	c.Set("id", task.UserId)
	c.Set("relay_mode", relayconstant.RelayModeImagesGenerations)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, task.Group)
	common.SetContextKey(c, constant.ContextKeyUserGroup, task.Group)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	return c, w, nil
}

func taskSubmitReqToImageRequest(req relaycommon.TaskSubmitReq, modelName string) (dto.ImageRequest, error) {
	if strings.TrimSpace(req.Model) == "" {
		req.Model = modelName
	}
	data, err := common.Marshal(req)
	if err != nil {
		return dto.ImageRequest{}, err
	}
	var imageReq dto.ImageRequest
	if err := common.Unmarshal(data, &imageReq); err != nil {
		return dto.ImageRequest{}, err
	}
	if strings.TrimSpace(imageReq.Model) == "" {
		imageReq.Model = modelName
	}
	return imageReq, nil
}

func applyAsyncImageResubmitResult(task *model.Task, result *TaskSubmitResult, info *relaycommon.RelayInfo, ch *model.Channel, tried map[int]struct{}, reason string) {
	task.ChannelId = ch.Id
	task.Platform = result.Platform
	task.Status = model.TaskStatusSubmitted
	task.Progress = "10%"
	task.StartTime = 0
	task.FinishTime = 0
	task.FailReason = ""
	task.Action = info.Action
	task.Data = result.TaskData
	task.Properties.OriginModelName = info.OriginModelName
	task.Properties.UpstreamModelName = info.UpstreamModelName
	task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
	task.PrivateData.ResultURL = ""
	task.PrivateData.ImageProtocol = result.ImageProtocol
	task.PrivateData.Key = info.ApiKey
	task.PrivateData.RetryCount++
	task.PrivateData.TriedChannelIDs = taskTriedChannelList(tried)
	task.PrivateData.LastFailureReason = strings.TrimSpace(reason)
	if result.InitialTaskInfo != nil {
		ApplyTaskInfoToTask(task, result.InitialTaskInfo, result.TaskData)
	}
	clearTerminalFireflyTaskKey(task, ch.Type)
}

func clearTerminalFireflyTaskKey(task *model.Task, channelType int) {
	if task == nil || channelType != constant.ChannelTypeFirefly {
		return
	}
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		task.PrivateData.Key = ""
	}
}

type syncImageStoredResponse struct {
	RequestID     string                 `json:"requestId,omitempty"`
	ModelCode     string                 `json:"modelCode,omitempty"`
	Status        string                 `json:"status,omitempty"`
	BillingStatus string                 `json:"billingStatus,omitempty"`
	Progress      int                    `json:"progress,omitempty"`
	ResultCount   int                    `json:"resultCount,omitempty"`
	Result        map[string]interface{} `json:"result,omitempty"`
	URL           string                 `json:"url,omitempty"`
	Error         *struct {
		Message string `json:"message,omitempty"`
		Code    string `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

func syncImageResponseToTaskInfo(taskID string, modelName string, responseBody []byte) (*relaycommon.TaskInfo, []byte, error) {
	var imageResp dto.ImageResponse
	if err := common.Unmarshal(responseBody, &imageResp); err != nil {
		return nil, nil, fmt.Errorf("unmarshal image response failed: %w", err)
	}
	saved, err := saveOpenAIImageResponseToOSS(imageResp)
	if err != nil {
		return nil, nil, err
	}
	if len(saved) == 0 {
		return nil, nil, fmt.Errorf("image response contains no result image")
	}

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
	stored := syncImageStoredResponse{
		RequestID:     taskID,
		ModelCode:     modelName,
		Status:        "succeeded",
		BillingStatus: "settled",
		Progress:      100,
		ResultCount:   len(saved),
		Result:        result,
		URL:           saved[0],
	}
	taskData, err := common.Marshal(stored)
	if err != nil {
		return nil, nil, err
	}
	return &relaycommon.TaskInfo{
		TaskID:   taskID,
		Status:   model.TaskStatusSuccess,
		Url:      saved[0],
		Progress: "100%",
		Data:     taskData,
	}, taskData, nil
}

func saveOpenAIImageResponseToOSS(imageResp dto.ImageResponse) ([]string, error) {
	saved := make([]string, 0, len(imageResp.Data))
	seen := make(map[string]struct{}, len(imageResp.Data))
	for _, image := range imageResp.Data {
		ossURL, err := saveOpenAIImageDataToOSS(image)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(ossURL) == "" {
			continue
		}
		if _, ok := seen[ossURL]; ok {
			continue
		}
		seen[ossURL] = struct{}{}
		saved = append(saved, ossURL)
	}
	return saved, nil
}

func saveOpenAIImageDataToOSS(image dto.ImageData) (string, error) {
	if strings.TrimSpace(image.Url) != "" {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(image.Url)), "data:") {
			return saveBase64ImageDataToOSS(image.Url)
		}
		return service.StrictSaveFileURLToAliyunOSS(image.Url, "")
	}
	if strings.TrimSpace(image.B64Json) != "" {
		return saveBase64ImageDataToOSS(image.B64Json)
	}
	return "", nil
}

func saveBase64ImageDataToOSS(data string) (string, error) {
	contentType, cleanBase64, err := service.DecodeBase64FileData(data)
	if err != nil {
		return "", err
	}
	ossURL, err := service.SaveBase64ImageToAliyunOSS(cleanBase64, contentType)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(ossURL) == "" {
		return "", fmt.Errorf("aliyun oss is not enabled or configured")
	}
	return ossURL, nil
}

func persistAsyncImageFailoverPrivateData(task *model.Task) error {
	return model.DB.Model(&model.Task{}).
		Where("id = ? AND status = ?", task.ID, task.Status).
		Update("private_data", task.PrivateData).Error
}

func taskTriedChannelSet(task *model.Task) map[int]struct{} {
	set := make(map[int]struct{}, len(task.PrivateData.TriedChannelIDs)+1)
	for _, id := range task.PrivateData.TriedChannelIDs {
		if id > 0 {
			set[id] = struct{}{}
		}
	}
	return set
}

func taskTriedChannelList(set map[int]struct{}) []int {
	ids := make([]int, 0, len(set))
	for id := range set {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids
}

func taskErrorToError(taskErr *dto.TaskError) error {
	if taskErr == nil {
		return nil
	}
	if taskErr.Error != nil {
		return taskErr.Error
	}
	if strings.TrimSpace(taskErr.Message) != "" {
		return fmt.Errorf("%s", taskErr.Message)
	}
	return fmt.Errorf("%s", taskErr.Code)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
