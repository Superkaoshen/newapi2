package mihuifang

import (
	"bytes"
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

type aiAPIProTaskResponse struct {
	TaskOrderID   int64                  `json:"taskOrderId,omitempty"`
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
	if !isSupportedModel(upstreamModel) {
		return nil, fmt.Errorf("unsupported model: %s", info.UpstreamModelName)
	}
	if isMultipartEditRequest(c, info) {
		return buildMultipartRequestBody(c, upstreamModel)
	}
	body := map[string]interface{}{
		"model":  upstreamModel,
		"prompt": req.Prompt,
	}
	if images := requestImages(req); len(images) > 0 {
		body["image"] = images
	}
	if len(req.ReferenceImages) > 0 {
		body["reference_images"] = req.ReferenceImages
	}
	setString(body, "size", req.Size)
	setString(body, "quality", req.Quality)
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

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	uri := fmt.Sprintf("%s/v1/tasks/%s", strings.TrimRight(baseUrl, "/"), taskID)
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
	var resp aiAPIProTaskResponse
	if err := common.Unmarshal(respBody, &resp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	ti := &relaycommon.TaskInfo{TaskID: resp.RequestID}
	status := strings.ToLower(resp.Status)
	switch status {
	case "pending", "queued", "submitted":
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

func buildMultipartRequestBody(c *gin.Context, upstreamModel string) (io.Reader, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("parse multipart form failed: %w", err)
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

	for key, values := range form.Value {
		if key == "model" {
			continue
		}
		switch key {
		case "images", "image[]":
			if err := writeValues("image", values); err != nil {
				return nil, err
			}
		case "referenceImages":
			if err := writeValues("reference_images", values); err != nil {
				return nil, err
			}
		default:
			if err := writeValues(key, values); err != nil {
				return nil, err
			}
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

func normalizeMultipartFileField(fieldName string) string {
	switch {
	case fieldName == "images" || fieldName == "image[]" || strings.HasPrefix(fieldName, "image["):
		return "image"
	case fieldName == "referenceImages":
		return "reference_images"
	default:
		return fieldName
	}
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
	upstreamModel = normalizeMihuifangModel(upstreamModel)
	if upstreamModel == "gpt-image-2" {
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
