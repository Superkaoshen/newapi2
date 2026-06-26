package mihuifang

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
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
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
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

const (
	imageTaskProtocolAIAPIPro = "aiapipro"
	imageTaskProtocolImageOne = "imageone"
	imageOneImageLimit        = 5
)

type aiAPIProTaskResponse struct {
	TaskOrderID   json.RawMessage        `json:"taskOrderId,omitempty"`
	RequestID     string                 `json:"requestId,omitempty"`
	ModelCode     string                 `json:"modelCode,omitempty"`
	ModelName     string                 `json:"modelName,omitempty"`
	Status        string                 `json:"status,omitempty"`
	BillingStatus string                 `json:"billingStatus,omitempty"`
	Progress      int                    `json:"progress,omitempty"`
	ResultCount   int                    `json:"resultCount,omitempty"`
	CreateTime    string                 `json:"createTime,omitempty"`
	Result        map[string]interface{} `json:"result,omitempty"`
	URL           string                 `json:"url,omitempty"`
	Error         *struct {
		Message string `json:"message,omitempty"`
		Code    string `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

type imageOneSubmitResponse struct {
	TaskID    string `json:"task_id,omitempty"`
	ID        string `json:"id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
}

type imageOneTaskResponse struct {
	TaskID  string              `json:"task_id,omitempty"`
	ID      string              `json:"id,omitempty"`
	Status  string              `json:"status,omitempty"`
	Images  []imageOneTaskImage `json:"images,omitempty"`
	Error   string              `json:"error,omitempty"`
	Message string              `json:"message,omitempty"`
}

type imageOneTaskImage struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
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
	if info.RelayMode == relayconstant.RelayModeImagesEdits || strings.Contains(c.Request.URL.Path, "/images/edits") {
		info.Action = constant.TaskActionGenerate
		return nil
	}
	info.Action = constant.TaskActionTextGenerate
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
	if _, ok := lookupConfiguredModelPrice(modelPriceCandidates(info.OriginModelName, info.UpstreamModelName)...); !ok {
		return fmt.Errorf("model price is required for %s", info.OriginModelName)
	}
	tierKey := imageTierPriceKey(req, info.UpstreamModelName)
	if _, ok := lookupConfiguredModelPrice(modelTierPriceCandidates(tierKey, info.OriginModelName, info.UpstreamModelName)...); !ok {
		return fmt.Errorf("model price is required for %s", info.OriginModelName+tierKey)
	}
	return nil
}

func (a *TaskAdaptor) ResolveBillingModelName(info *relaycommon.RelayInfo) string {
	if info == nil {
		return ""
	}
	for _, name := range modelPriceCandidates(info.OriginModelName, info.UpstreamModelName) {
		if _, ok := lookupConfiguredModelPrice(name); ok {
			return name
		}
	}
	return info.OriginModelName
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if mihuifangImageProtocol(info) == imageTaskProtocolImageOne {
		return strings.TrimRight(a.baseURL, "/") + "/v1/images/edits", nil
	}
	path := "/v1/images/generations"
	if info.RelayMode == relayconstant.RelayModeImagesEdits || strings.Contains(info.RequestURLPath, "/images/edits") {
		path = "/v1/images/edits"
	}
	return strings.TrimRight(a.baseURL, "/") + path, nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	if contentType := c.Request.Header.Get("Content-Type"); strings.HasPrefix(contentType, "multipart/form-data") {
		req.Header.Set("Content-Type", contentType)
	} else {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	upstreamModel := normalizeMihuifangModel(info.UpstreamModelName)
	if mihuifangImageProtocol(info) == imageTaskProtocolImageOne {
		return buildImageOneRequestBody(c, upstreamModel, req)
	}
	if !isSupportedModel(upstreamModel) {
		return nil, fmt.Errorf("unsupported model: %s", info.UpstreamModelName)
	}
	if isMultipartEditRequest(c, info) {
		return buildMultipartRequestBody(c, upstreamModel)
	}
	images := requestImages(req)
	if err := validateImageInputLimit(upstreamModel, len(images)+len(req.ReferenceImages)); err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"model":  upstreamModel,
		"prompt": req.Prompt,
	}
	if len(images) > 0 {
		body["image"] = images
	}
	if len(req.ReferenceImages) > 0 {
		body["reference_images"] = req.ReferenceImages
	}
	if err := setImageRequestOptions(body, upstreamModel, req); err != nil {
		return nil, err
	}
	setString(body, "response_format", req.ResponseFormat)
	if len(req.Mask) > 0 {
		body["mask"] = req.Mask
	}
	if req.OutputPSD != nil {
		body["output_psd"] = *req.OutputPSD
	}
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

	if mihuifangImageProtocol(info) == imageTaskProtocolImageOne {
		return a.doImageOneResponse(c, responseBody, info)
	}

	var upstream aiAPIProTaskResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}

	upstreamID := upstream.RequestID
	if upstreamID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("requestId is empty"), "invalid_response", http.StatusInternalServerError)
	}

	upstream.RequestID = info.PublicTaskID
	sanitizePublicResponseModel(&upstream, info.OriginModelName)
	if upstream.Status == "" {
		upstream.Status = "submitted"
	}
	if isSuccessStatus(upstream.Status) {
		saved, err := saveResultFilesToOSS(upstream.Result, upstream.URL)
		if err != nil {
			return "", nil, service.TaskErrorWrapper(err, "save_result_file_failed", http.StatusInternalServerError)
		}
		upstream = applySavedImages(upstream, saved)
	}

	c.JSON(http.StatusOK, upstream)
	taskData := normalizeResponseData(upstream)
	return upstreamID, taskData, nil
}

func (a *TaskAdaptor) doImageOneResponse(c *gin.Context, responseBody []byte, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	var upstream imageOneSubmitResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	upstreamID := firstNonEmpty(upstream.TaskID, upstream.ID, upstream.RequestID)
	if upstreamID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}
	if upstream.Error != "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("%s", upstream.Error), "upstream_task_failed", http.StatusBadGateway)
	}

	publicResp := aiAPIProTaskResponse{
		RequestID:     info.PublicTaskID,
		ModelCode:     info.OriginModelName,
		Status:        normalizeImageOneSubmitStatus(upstream.Status),
		BillingStatus: "pending",
		Progress:      20,
	}
	c.JSON(http.StatusOK, publicResp)
	return upstreamID, normalizeResponseData(publicResp), nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	path := "/v1/tasks/%s"
	if taskBodyImageProtocol(body) == imageTaskProtocolImageOne {
		path = "/v1/status/%s"
	}
	uri := fmt.Sprintf(strings.TrimRight(baseUrl, "/")+path, taskID)
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
	if isImageOneTaskResponse(respBody) {
		return parseImageOneTaskResult(respBody)
	}

	var resp aiAPIProTaskResponse
	if err := common.Unmarshal(respBody, &resp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	ti := &relaycommon.TaskInfo{TaskID: resp.RequestID}
	status := strings.ToLower(resp.Status)
	switch status {
	case "created", "pending", "queued", "submitted":
		ti.Status = model.TaskStatusQueued
	case "processing", "in_progress", "running":
		ti.Status = model.TaskStatusInProgress
	case "completed", "succeeded", "success":
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
		saved, err := saveResultFilesToOSS(resp.Result, resp.URL)
		if err != nil {
			ti.Status = model.TaskStatusFailure
			ti.Reason = fmt.Sprintf("save result file to aliyun oss failed: %s", err.Error())
			ti.Progress = "100%"
			ti.Data = failureResponseData(resp, ti.Reason)
			return ti, nil
		}
		resp = applySavedImages(resp, saved)
		ti.Url = firstNonEmpty(resp.URL, firstResultURL(resp.Result))
		ti.Data = normalizeResponseData(resp)
	case "failed", "failure", "timeout", "cancelled", "canceled":
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

func normalizeImageOneSubmitStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "succeeded", "success":
		return "succeeded"
	case "failed", "failure", "timeout", "cancelled", "canceled":
		return "failed"
	default:
		return "submitted"
	}
}

func isImageOneTaskResponse(respBody []byte) bool {
	var probe struct {
		Images json.RawMessage `json:"images,omitempty"`
		Error  string          `json:"error,omitempty"`
	}
	if err := common.Unmarshal(respBody, &probe); err != nil {
		return false
	}
	if len(probe.Images) > 0 || probe.Error != "" {
		return true
	}
	return false
}

func parseImageOneTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var resp imageOneTaskResponse
	if err := common.Unmarshal(respBody, &resp); err != nil {
		return nil, errors.Wrap(err, "unmarshal imageone task result failed")
	}

	ti := &relaycommon.TaskInfo{TaskID: firstNonEmpty(resp.TaskID, resp.ID)}
	switch strings.ToLower(strings.TrimSpace(resp.Status)) {
	case "pending":
		ti.Status = model.TaskStatusQueued
	case "processing":
		ti.Status = model.TaskStatusInProgress
	case "completed":
		ti.Status = model.TaskStatusSuccess
		ti.Progress = "100%"
		publicResp, err := imageOneResponseToAIAPIPro(resp)
		if err != nil {
			ti.Status = model.TaskStatusFailure
			ti.Reason = err.Error()
			ti.Data = failureResponseData(aiAPIProTaskResponse{RequestID: ti.TaskID}, ti.Reason)
			return ti, nil
		}
		ti.Url = firstResultURL(publicResp.Result)
		ti.Data = normalizeResponseData(publicResp)
	case "failed":
		ti.Status = model.TaskStatusFailure
		ti.Progress = "100%"
		ti.Reason = firstNonEmpty(resp.Error, resp.Message, "task failed")
	default:
		ti.Status = model.TaskStatusInProgress
	}
	if ti.Data == nil {
		publicResp := aiAPIProTaskResponse{
			RequestID: ti.TaskID,
			Status:    imageOneStatusToPublicStatus(resp.Status),
			Progress:  imageOneProgress(resp.Status),
		}
		if ti.Status == model.TaskStatusFailure {
			publicResp.Error = &struct {
				Message string `json:"message,omitempty"`
				Code    string `json:"code,omitempty"`
			}{Message: ti.Reason, Code: "upstream_task_failed"}
		}
		ti.Data = normalizeResponseData(publicResp)
	}
	return ti, nil
}

func imageOneResponseToAIAPIPro(resp imageOneTaskResponse) (aiAPIProTaskResponse, error) {
	saved, err := saveImageOneImagesToOSS(resp.Images)
	if err != nil {
		return aiAPIProTaskResponse{}, err
	}
	if len(saved) == 0 {
		return aiAPIProTaskResponse{}, fmt.Errorf("completed response contains no result image")
	}
	items := make([]interface{}, 0, len(saved))
	for _, url := range saved {
		items = append(items, map[string]interface{}{
			"url":  url,
			"type": "image",
		})
	}
	result := map[string]interface{}{
		"items":      items,
		"image_url":  saved[0],
		"image_urls": saved,
		"url":        saved[0],
	}
	if len(saved) == 1 {
		delete(result, "image_urls")
	}
	return aiAPIProTaskResponse{
		RequestID:   firstNonEmpty(resp.TaskID, resp.ID),
		Status:      "succeeded",
		Progress:    100,
		ResultCount: len(saved),
		Result:      result,
		URL:         saved[0],
	}, nil
}

func saveImageOneImagesToOSS(images []imageOneTaskImage) ([]string, error) {
	saved := make([]string, 0, len(images))
	for _, image := range images {
		if strings.TrimSpace(image.URL) != "" {
			ossURL, err := service.StrictSaveFileURLToAliyunOSS(image.URL, "")
			if err != nil {
				return nil, err
			}
			saved = append(saved, ossURL)
			continue
		}
		if strings.TrimSpace(image.B64JSON) != "" {
			contentType, cleanBase64, err := service.DecodeBase64FileData(image.B64JSON)
			if err != nil {
				return nil, err
			}
			ossURL, err := service.SaveBase64ImageToAliyunOSS(cleanBase64, contentType)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(ossURL) == "" {
				return nil, fmt.Errorf("aliyun oss is not enabled or configured")
			}
			saved = append(saved, ossURL)
		}
	}
	return uniqueNonEmptyStrings(saved), nil
}

func imageOneStatusToPublicStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return "created"
	case "processing":
		return "processing"
	case "completed":
		return "succeeded"
	case "failed":
		return "failed"
	default:
		return "processing"
	}
}

func imageOneProgress(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return 0
	case "processing":
		return 50
	case "completed", "failed":
		return 100
	default:
		return 0
	}
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func isSuccessStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "succeeded", "success":
		return true
	default:
		return false
	}
}

func isSupportedModel(modelName string) bool {
	for _, m := range ModelList {
		if modelName == m {
			return true
		}
	}
	return false
}

func normalizeMihuifangModel(modelName string) string {
	switch strings.TrimSpace(modelName) {
	case "nano-banana":
		return "nanobanana"
	case "nano-banana2":
		return "nanobanana2"
	case "nano-banana-pro":
		return "nanobananapro"
	default:
		return strings.TrimSpace(modelName)
	}
}

func mihuifangModelFamily(modelName string) string {
	switch normalizeMihuifangModel(modelName) {
	case "nanobanana-5":
		return "nanobanana"
	case "nanobanana2-5":
		return "nanobanana2"
	case "nanobananapro-5":
		return "nanobananapro"
	default:
		return normalizeMihuifangModel(modelName)
	}
}

func mihuifangImageProtocol(info *relaycommon.RelayInfo) string {
	if info == nil {
		return imageTaskProtocolAIAPIPro
	}
	protocol := strings.ToLower(strings.TrimSpace(info.ChannelOtherSettings.ImageTaskProtocol))
	if protocol == "" {
		return imageTaskProtocolAIAPIPro
	}
	return protocol
}

func taskBodyImageProtocol(body map[string]any) string {
	raw, _ := body["image_protocol"].(string)
	protocol := strings.ToLower(strings.TrimSpace(raw))
	if protocol == "" {
		return imageTaskProtocolAIAPIPro
	}
	return protocol
}

func isMultipartEditRequest(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if info.RelayMode != relayconstant.RelayModeImagesEdits && !strings.Contains(c.Request.URL.Path, "/images/edits") {
		return false
	}
	return strings.HasPrefix(c.Request.Header.Get("Content-Type"), "multipart/form-data")
}

func requestImages(req relaycommon.TaskSubmitReq) []string {
	images := make([]string, 0, len(req.Images)+1)
	if strings.TrimSpace(req.Image) != "" {
		images = append(images, strings.TrimSpace(req.Image))
	}
	for _, image := range req.Images {
		image = strings.TrimSpace(image)
		if image != "" {
			images = append(images, image)
		}
	}
	return uniqueStrings(images)
}

type aiAPIProSizeSpec struct {
	Aspect   string
	Tier     string
	Size     string
	Extended bool
}

var aiAPIProSizeSpecs = []aiAPIProSizeSpec{
	{Aspect: "21:9", Tier: "1k", Size: "1584x672"},
	{Aspect: "16:9", Tier: "1k", Size: "1376x768"},
	{Aspect: "3:2", Tier: "1k", Size: "1264x848"},
	{Aspect: "4:3", Tier: "1k", Size: "1200x896"},
	{Aspect: "5:4", Tier: "1k", Size: "1152x928"},
	{Aspect: "1:1", Tier: "1k", Size: "1024x1024"},
	{Aspect: "4:5", Tier: "1k", Size: "928x1152"},
	{Aspect: "3:4", Tier: "1k", Size: "896x1200"},
	{Aspect: "2:3", Tier: "1k", Size: "848x1264"},
	{Aspect: "9:16", Tier: "1k", Size: "768x1376"},
	{Aspect: "21:9", Tier: "2k", Size: "3168x1344"},
	{Aspect: "16:9", Tier: "2k", Size: "2752x1536"},
	{Aspect: "3:2", Tier: "2k", Size: "2528x1696"},
	{Aspect: "4:3", Tier: "2k", Size: "2400x1792"},
	{Aspect: "5:4", Tier: "2k", Size: "2304x1856"},
	{Aspect: "1:1", Tier: "2k", Size: "2048x2048"},
	{Aspect: "4:5", Tier: "2k", Size: "1856x2304"},
	{Aspect: "3:4", Tier: "2k", Size: "1792x2400"},
	{Aspect: "2:3", Tier: "2k", Size: "1696x2528"},
	{Aspect: "9:16", Tier: "2k", Size: "1536x2752"},
	{Aspect: "21:9", Tier: "4k", Size: "6336x2688"},
	{Aspect: "16:9", Tier: "4k", Size: "5504x3072"},
	{Aspect: "3:2", Tier: "4k", Size: "5056x3392"},
	{Aspect: "4:3", Tier: "4k", Size: "4800x3584"},
	{Aspect: "5:4", Tier: "4k", Size: "4608x3712"},
	{Aspect: "1:1", Tier: "4k", Size: "4096x4096"},
	{Aspect: "4:5", Tier: "4k", Size: "3712x4608"},
	{Aspect: "3:4", Tier: "4k", Size: "3584x4800"},
	{Aspect: "2:3", Tier: "4k", Size: "3392x5056"},
	{Aspect: "9:16", Tier: "4k", Size: "3072x5504"},
	{Aspect: "8:1", Tier: "1k", Size: "3072x384", Extended: true},
	{Aspect: "4:1", Tier: "1k", Size: "2048x512", Extended: true},
	{Aspect: "1:4", Tier: "1k", Size: "512x2048", Extended: true},
	{Aspect: "1:8", Tier: "1k", Size: "384x3072", Extended: true},
	{Aspect: "8:1", Tier: "2k", Size: "6144x768", Extended: true},
	{Aspect: "4:1", Tier: "2k", Size: "4096x1024", Extended: true},
	{Aspect: "1:4", Tier: "2k", Size: "1024x4096", Extended: true},
	{Aspect: "1:8", Tier: "2k", Size: "768x6144", Extended: true},
	{Aspect: "8:1", Tier: "4k", Size: "12288x1536", Extended: true},
	{Aspect: "4:1", Tier: "4k", Size: "8192x2048", Extended: true},
	{Aspect: "1:4", Tier: "4k", Size: "2048x8192", Extended: true},
	{Aspect: "1:8", Tier: "4k", Size: "1536x12288", Extended: true},
}

var aiAPIProSizeByPixels, aiAPIProSizeByAspectTier = buildAIAPIProSizeIndexes()

func buildAIAPIProSizeIndexes() (map[string]aiAPIProSizeSpec, map[string]map[string]aiAPIProSizeSpec) {
	byPixels := make(map[string]aiAPIProSizeSpec, len(aiAPIProSizeSpecs))
	byAspectTier := make(map[string]map[string]aiAPIProSizeSpec, len(aiAPIProSizeSpecs))
	for _, spec := range aiAPIProSizeSpecs {
		byPixels[spec.Size] = spec
		if byAspectTier[spec.Aspect] == nil {
			byAspectTier[spec.Aspect] = make(map[string]aiAPIProSizeSpec)
		}
		byAspectTier[spec.Aspect][spec.Tier] = spec
	}
	return byPixels, byAspectTier
}

func setImageRequestOptions(body map[string]interface{}, upstreamModel string, req relaycommon.TaskSubmitReq) error {
	size, quality, err := normalizedImageRequestOptions(upstreamModel, req)
	if err != nil {
		return err
	}
	setString(body, "size", size)
	setString(body, "quality", quality)
	return nil
}

func normalizedImageRequestOptions(upstreamModel string, req relaycommon.TaskSubmitReq) (string, string, error) {
	modelFamily := mihuifangModelFamily(upstreamModel)
	size, tier, err := normalizeUpstreamImageSize(upstreamModel, req.Size, req.AspectRatio, req.Resolution)
	if err != nil {
		return "", "", err
	}
	quality := strings.ToLower(strings.TrimSpace(req.Quality))
	if modelFamily == "gpt-image-2" {
		if quality != "" && quality != "low" && quality != "medium" && quality != "high" {
			return "", "", fmt.Errorf("unsupported quality for gpt-image-2: %s", req.Quality)
		}
		return size, quality, nil
	}
	if size != "" {
		return size, nanoQualityForTier(tier), nil
	}
	if quality != "" && quality != "standard" && quality != "hd" {
		return "", "", fmt.Errorf("unsupported quality for %s: %s", modelFamily, req.Quality)
	}
	return "", quality, nil
}

func normalizeUpstreamImageSize(upstreamModel, size, aspectRatio, resolution string) (string, string, error) {
	modelFamily := mihuifangModelFamily(upstreamModel)
	rawSize := strings.TrimSpace(size)
	rawAspect := strings.TrimSpace(aspectRatio)
	rawResolution := strings.TrimSpace(resolution)
	if rawSize == "" && rawAspect == "" && rawResolution == "" {
		return "", "", nil
	}
	if strings.EqualFold(rawSize, "auto") && rawAspect == "" && rawResolution == "" {
		return "", "", nil
	}
	if w, h, ok := parsePixels(rawSize); ok {
		pixelSize := fmt.Sprintf("%dx%d", w, h)
		if spec, ok := aiAPIProSizeByPixels[pixelSize]; ok {
			if !isImageSizeSupportedByModel(upstreamModel, spec) {
				return "", "", fmt.Errorf("%s does not support ratio %s", modelFamily, spec.Aspect)
			}
			return pixelSize, spec.Tier, nil
		}
		if modelFamily == "gpt-image-2" {
			return pixelSize, "", nil
		}
		return "", "", fmt.Errorf("unsupported size for %s: %s", modelFamily, rawSize)
	}

	tier := imageTierFromText(firstNonEmpty(rawSize, rawResolution))
	aspect := firstNonEmpty(imageAspectFromText(rawSize), imageAspectFromText(rawAspect))
	if aspect == "" && tier != "" {
		aspect = "1:1"
	}
	if tier == "" {
		tier = "1k"
	}
	if aspect == "" {
		return "", "", fmt.Errorf("unsupported size for %s: %s", modelFamily, firstNonEmpty(rawSize, rawAspect, rawResolution))
	}
	specsByTier, ok := aiAPIProSizeByAspectTier[aspect]
	if !ok {
		return "", "", fmt.Errorf("unsupported ratio for %s: %s", modelFamily, aspect)
	}
	spec, ok := specsByTier[tier]
	if !ok {
		return "", "", fmt.Errorf("unsupported size tier for %s: %s", modelFamily, tier)
	}
	if !isImageSizeSupportedByModel(upstreamModel, spec) {
		return "", "", fmt.Errorf("%s does not support ratio %s", modelFamily, spec.Aspect)
	}
	return spec.Size, spec.Tier, nil
}

func isImageSizeSupportedByModel(upstreamModel string, spec aiAPIProSizeSpec) bool {
	modelFamily := mihuifangModelFamily(upstreamModel)
	if modelFamily == "gpt-image-2" || modelFamily == "nanobanana2" {
		return true
	}
	return !spec.Extended
}

func nanoQualityForTier(tier string) string {
	if strings.EqualFold(tier, "1k") {
		return "standard"
	}
	return "hd"
}

func validateImageInputLimit(upstreamModel string, count int) error {
	limit := imageInputLimit(upstreamModel)
	if limit <= 0 || count <= limit {
		return nil
	}
	return fmt.Errorf("%s supports at most %d input images", mihuifangModelFamily(upstreamModel), limit)
}

func imageInputLimit(upstreamModel string) int {
	switch mihuifangModelFamily(upstreamModel) {
	case "nanobanana":
		return 4
	case "nanobanana2", "nanobananapro", "gpt-image-2":
		return 6
	default:
		return 0
	}
}

func buildImageOneRequestBody(c *gin.Context, upstreamModel string, req relaycommon.TaskSubmitReq) (io.Reader, error) {
	if strings.TrimSpace(upstreamModel) == "" {
		return nil, fmt.Errorf("model field is required")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("prompt field is required")
	}
	if isMultipartEditRequest(c, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}) {
		return buildImageOneMultipartRequestBody(c, upstreamModel)
	}
	return buildImageOneJSONRequestBody(upstreamModel, req)
}

func buildImageOneJSONRequestBody(upstreamModel string, req relaycommon.TaskSubmitReq) (io.Reader, error) {
	referenceURLs := imageOneReferenceURLsFromTask(req)
	if len(referenceURLs) == 0 {
		return nil, fmt.Errorf("image or reference_image_urls is required")
	}
	if len(referenceURLs) > imageOneImageLimit {
		return nil, fmt.Errorf("imageone supports at most %d input images", imageOneImageLimit)
	}

	body := map[string]interface{}{
		"model":                upstreamModel,
		"prompt":               req.Prompt,
		"reference_image_urls": referenceURLs,
	}
	setImageOneOptions(body, req.Size, req.AspectRatio, req.Resolution, req.ResponseFormat)
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func buildImageOneMultipartRequestBody(c *gin.Context, upstreamModel string) (io.Reader, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("parse multipart form failed: %w", err)
	}

	fields := normalizeImageOneMultipartFields(form.Value)
	if err := validateImageOneMultipartInputs(fields, form.File); err != nil {
		return nil, err
	}

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	if err := writer.WriteField("model", upstreamModel); err != nil {
		return nil, err
	}

	writeValues := func(field string, values []string) error {
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				continue
			}
			if err := writer.WriteField(field, value); err != nil {
				return err
			}
		}
		return nil
	}
	for key, values := range fields {
		if err := writeValues(key, values); err != nil {
			return nil, err
		}
	}
	for fieldName, files := range form.File {
		upstreamField := normalizeImageOneMultipartFileField(fieldName)
		for _, fileHeader := range files {
			if err := copyMultipartFile(writer, upstreamField, fileHeader); err != nil {
				return nil, err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return &requestBody, nil
}

func normalizeImageOneMultipartFields(values map[string][]string) map[string][]string {
	fields := make(map[string][]string, len(values))
	for key, fieldValues := range values {
		if key == "model" {
			continue
		}
		upstreamField := normalizeImageOneMultipartValueField(key)
		fields[upstreamField] = append(fields[upstreamField], fieldValues...)
	}

	size := firstFieldValue(fields, "size")
	aspect := firstFieldValue(fields, "aspect_ratio")
	resolution := firstFieldValue(fields, "resolution")
	responseFormat := firstFieldValue(fields, "response_format")
	delete(fields, "size")
	delete(fields, "aspect_ratio")
	delete(fields, "resolution")
	delete(fields, "response_format")

	options := map[string]interface{}{}
	setImageOneOptions(options, size, aspect, resolution, responseFormat)
	if v, ok := options["aspect_ratio"].(string); ok && v != "" {
		fields["aspect_ratio"] = []string{v}
	}
	if v, ok := options["resolution"].(string); ok && v != "" {
		fields["resolution"] = []string{v}
	}
	if v, ok := options["response_format"].(string); ok && v != "" {
		fields["response_format"] = []string{v}
	}
	return fields
}

func normalizeImageOneMultipartValueField(fieldName string) string {
	switch {
	case fieldName == "image_urls" || fieldName == "image" || fieldName == "images" || fieldName == "referenceImages" || fieldName == "input_images":
		return "reference_image_urls"
	default:
		return fieldName
	}
}

func normalizeImageOneMultipartFileField(fieldName string) string {
	switch {
	case fieldName == "image[]" || strings.HasPrefix(fieldName, "image["):
		return "images"
	default:
		return fieldName
	}
}

func validateImageOneMultipartInputs(fields map[string][]string, files map[string][]*multipart.FileHeader) error {
	if strings.TrimSpace(firstFieldValue(fields, "prompt")) == "" {
		return fmt.Errorf("prompt field is required")
	}
	fileCount := 0
	for fieldName, fieldFiles := range files {
		switch normalizeImageOneMultipartFileField(fieldName) {
		case "image", "images":
			fileCount += len(fieldFiles)
		}
	}
	referenceURLCount := countNonEmptyStrings(fields["reference_image_urls"])
	if fileCount+referenceURLCount == 0 {
		return fmt.Errorf("image or reference_image_urls is required")
	}
	if fileCount+referenceURLCount > imageOneImageLimit {
		return fmt.Errorf("imageone supports at most %d input images", imageOneImageLimit)
	}
	return nil
}

func imageOneReferenceURLsFromTask(req relaycommon.TaskSubmitReq) []string {
	referenceURLs := requestImages(req)
	referenceURLs = append(referenceURLs, req.ReferenceImages...)
	referenceURLs = append(referenceURLs, req.ReferenceImageURLs...)
	if req.Metadata != nil {
		referenceURLs = append(referenceURLs, metadataStringSlice(req.Metadata["reference_image_urls"])...)
	}
	return uniqueNonEmptyStrings(referenceURLs)
}

func metadataStringSlice(v interface{}) []string {
	switch value := v.(type) {
	case []string:
		return value
	case []interface{}:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{value}
	default:
		return nil
	}
}

func setImageOneOptions(body map[string]interface{}, size, aspectRatio, resolution, responseFormat string) {
	aspect := firstNonEmpty(imageAspectFromText(aspectRatio), imageAspectFromText(size))
	if aspect != "" {
		body["aspect_ratio"] = aspect
	}
	tier := strings.ToUpper(imageTierFromText(firstNonEmpty(resolution, size)))
	if tier != "" {
		body["resolution"] = tier
	}
	format := normalizeImageOneResponseFormat(responseFormat)
	if format != "" {
		body["response_format"] = format
	}
}

func normalizeImageOneResponseFormat(responseFormat string) string {
	switch strings.ToLower(strings.TrimSpace(responseFormat)) {
	case "b64_json", "base64":
		return "base64"
	case "url":
		return "url"
	case "":
		return "url"
	default:
		return strings.TrimSpace(responseFormat)
	}
}

func buildMultipartRequestBody(c *gin.Context, upstreamModel string) (io.Reader, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("parse multipart form failed: %w", err)
	}
	fields, err := normalizeMultipartFields(form.Value, upstreamModel)
	if err != nil {
		return nil, err
	}
	if err := validateImageInputLimit(upstreamModel, countMultipartImageInputs(fields, form.File)); err != nil {
		return nil, err
	}

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	if err := writer.WriteField("model", upstreamModel); err != nil {
		return nil, err
	}

	writeValues := func(field string, values []string) error {
		for _, value := range values {
			if err := writer.WriteField(field, value); err != nil {
				return err
			}
		}
		return nil
	}

	for key, values := range fields {
		if err := writeValues(key, values); err != nil {
			return nil, err
		}
	}

	for fieldName, files := range form.File {
		upstreamField := normalizeMultipartFileField(fieldName)
		for _, fileHeader := range files {
			if err := copyMultipartFile(writer, upstreamField, fileHeader); err != nil {
				return nil, err
			}
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return &requestBody, nil
}

func normalizeMultipartFields(values map[string][]string, upstreamModel string) (map[string][]string, error) {
	fields := make(map[string][]string, len(values))
	for key, fieldValues := range values {
		if key == "model" {
			continue
		}
		upstreamField := normalizeMultipartValueField(key)
		fields[upstreamField] = append(fields[upstreamField], fieldValues...)
	}
	req := relaycommon.TaskSubmitReq{
		Size:           firstFieldValue(fields, "size"),
		Quality:        firstFieldValue(fields, "quality"),
		AspectRatio:    firstFieldValue(fields, "aspect_ratio"),
		Resolution:     firstFieldValue(fields, "resolution"),
		ResponseFormat: firstFieldValue(fields, "response_format"),
	}
	delete(fields, "size")
	delete(fields, "quality")
	delete(fields, "aspect_ratio")
	delete(fields, "resolution")

	size, quality, err := normalizedImageRequestOptions(upstreamModel, req)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(size) != "" {
		fields["size"] = []string{size}
	}
	if strings.TrimSpace(quality) != "" {
		fields["quality"] = []string{quality}
	}
	return fields, nil
}

func normalizeMultipartValueField(fieldName string) string {
	switch {
	case fieldName == "images" || fieldName == "image[]" || strings.HasPrefix(fieldName, "image["):
		return "image"
	case fieldName == "referenceImages" || fieldName == "input_images":
		return "reference_images"
	default:
		return fieldName
	}
}

func normalizeMultipartFileField(fieldName string) string {
	switch {
	case fieldName == "images" || fieldName == "image[]" || strings.HasPrefix(fieldName, "image["):
		return "image"
	case fieldName == "referenceImages" || fieldName == "input_images":
		return "reference_images"
	default:
		return fieldName
	}
}

func firstFieldValue(fields map[string][]string, key string) string {
	for _, value := range fields[key] {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func countMultipartImageInputs(fields map[string][]string, files map[string][]*multipart.FileHeader) int {
	count := countNonEmptyStrings(fields["image"]) + countNonEmptyStrings(fields["reference_images"])
	for fieldName, fieldFiles := range files {
		switch normalizeMultipartFileField(fieldName) {
		case "image", "reference_images":
			count += len(fieldFiles)
		}
	}
	return count
}

func countNonEmptyStrings(values []string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func copyMultipartFile(writer *multipart.Writer, fieldName string, fileHeader *multipart.FileHeader) error {
	file, err := fileHeader.Open()
	if err != nil {
		return fmt.Errorf("open multipart file %q failed: %w", fileHeader.Filename, err)
	}
	defer file.Close()

	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileHeader.Filename))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create multipart part %q failed: %w", fieldName, err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copy multipart file %q failed: %w", fileHeader.Filename, err)
	}
	return nil
}

func setString(m map[string]interface{}, key, value string) {
	if strings.TrimSpace(value) != "" {
		m[key] = value
	}
}

func saveResultFilesToOSS(result map[string]interface{}, topURL string) ([]string, error) {
	raw := collectResultURLs(result, topURL)
	if len(raw) == 0 {
		return nil, fmt.Errorf("completed response contains no result url")
	}
	saved := make([]string, 0, len(raw))
	for _, u := range raw {
		ossURL, err := service.StrictSaveFileURLToAliyunOSS(u, "")
		if err != nil {
			return nil, err
		}
		saved = append(saved, ossURL)
	}
	return saved, nil
}

func collectResultURLs(result map[string]interface{}, topURL string) []string {
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
		if items, ok := result["items"].([]interface{}); ok {
			for _, item := range items {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if v, ok := itemMap["url"].(string); ok {
					add(v)
				}
			}
		}
	}
	add(topURL)
	return uniqueStrings(urls)
}

func applySavedImages(resp aiAPIProTaskResponse, saved []string) aiAPIProTaskResponse {
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
	if items, ok := resp.Result["items"].([]interface{}); ok {
		savedIndex := 0
		for i, item := range items {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if _, ok := itemMap["url"].(string); !ok {
				continue
			}
			if savedIndex >= len(saved) {
				break
			}
			itemMap["url"] = saved[savedIndex]
			items[i] = itemMap
			savedIndex++
		}
		resp.Result["items"] = items
	}
	resp.Status = "succeeded"
	resp.Progress = 100
	resp.ResultCount = len(saved)
	return resp
}

func firstResultURL(result map[string]interface{}) string {
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
	if items, ok := result["items"].([]interface{}); ok {
		for _, item := range items {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if s, ok := itemMap["url"].(string); ok {
				return s
			}
		}
	}
	return ""
}

func normalizeResponseData(resp aiAPIProTaskResponse) []byte {
	clearUpstreamModelName(&resp)
	data, _ := common.Marshal(resp)
	return data
}

func failureResponseData(resp aiAPIProTaskResponse, reason string) []byte {
	clearUpstreamModelName(&resp)
	resp.Result = nil
	resp.URL = ""
	resp.Status = "failed"
	resp.Progress = 100
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

func uniqueNonEmptyStrings(values []string) []string {
	clean := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			clean = append(clean, v)
		}
	}
	return uniqueStrings(clean)
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
	basePrice, ok := lookupConfiguredModelPrice(modelPriceCandidates(originModel, upstreamModel)...)
	if !ok || basePrice <= 0 {
		return 1
	}
	tierKey := imageTierPriceKey(req, upstreamModel)
	tierPrice, ok := lookupConfiguredModelPrice(modelTierPriceCandidates(tierKey, originModel, upstreamModel)...)
	if !ok || tierPrice <= 0 {
		return 1
	}
	return tierPrice / basePrice
}

func imageTierPriceKey(req relaycommon.TaskSubmitReq, upstreamModel string) string {
	modelFamily := mihuifangModelFamily(upstreamModel)
	if modelFamily == "gpt-image-2" {
		quality := strings.ToLower(strings.TrimSpace(req.Quality))
		if quality == "" {
			quality = "low"
		}
		parts := []string{quality}
		if req.OutputPSD != nil && *req.OutputPSD {
			parts = append(parts, "psd")
		}
		return "@" + strings.Join(parts, "@")
	}
	return "@" + imageSizeTier(req.Size, req.Resolution)
}

func modelPriceCandidates(originModel, upstreamModel string) []string {
	names := make([]string, 0, 4)
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		names = append(names, name)
		normalized := normalizeMihuifangModel(name)
		if normalized != name {
			names = append(names, normalized)
		}
		family := mihuifangModelFamily(name)
		if family != name && family != normalized {
			names = append(names, family)
		}
	}
	add(originModel)
	add(upstreamModel)
	return uniqueStrings(names)
}

func modelTierPriceCandidates(tierKey, originModel, upstreamModel string) []string {
	baseNames := modelPriceCandidates(originModel, upstreamModel)
	names := make([]string, 0, len(baseNames))
	for _, name := range baseNames {
		names = append(names, name+tierKey)
	}
	return names
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
	text := strings.ToLower(strings.TrimSpace(firstNonEmpty(size, resolution)))
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
		if spec, ok := aiAPIProSizeByPixels[fmt.Sprintf("%dx%d", w, h)]; ok {
			return spec.Tier
		}
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
var aspectRatioRe = regexp.MustCompile(`(?i)(^|[^0-9])(\d{1,2})\s*[:x]\s*(\d{1,2})([^0-9]|$)`)

func imageTierFromText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(text, "4k"):
		return "4k"
	case strings.Contains(text, "2k"):
		return "2k"
	case strings.Contains(text, "1k"):
		return "1k"
	default:
		return ""
	}
}

func imageAspectFromText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	matches := aspectRatioRe.FindStringSubmatch(text)
	if len(matches) != 5 {
		return ""
	}
	return matches[2] + ":" + matches[3]
}

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
		var resp aiAPIProTaskResponse
		if err := common.Unmarshal(task.Data, &resp); err == nil {
			resp.RequestID = task.TaskID
			sanitizePublicResponseModel(&resp, publicTaskModel(task))
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
	resp := aiAPIProTaskResponse{
		RequestID: task.TaskID,
		ModelCode: publicTaskModel(task),
	}
	clearUpstreamModelName(&resp)
	applyTaskStatus(&resp, task)
	return normalizeResponseData(resp)
}

func sanitizePublicResponseModel(resp *aiAPIProTaskResponse, publicModel string) {
	resp.ModelCode = publicModel
	clearUpstreamModelName(resp)
}

func clearUpstreamModelName(resp *aiAPIProTaskResponse) {
	resp.ModelName = ""
}

func publicTaskModel(task *model.Task) string {
	if task == nil {
		return ""
	}
	if task.Properties.OriginModelName != "" {
		return task.Properties.OriginModelName
	}
	return task.Properties.UpstreamModelName
}

func applyTaskStatus(resp *aiAPIProTaskResponse, task *model.Task) {
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
