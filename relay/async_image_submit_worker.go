package relay

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

const (
	asyncImageSubmitBatchSize       = 100
	asyncImageSubmitWindowSize      = 10
	asyncImageSubmitInterval        = 2 * time.Second
	asyncImageSubmitClaimTimeout    = 2 * time.Minute
	asyncImageFireflySubmitTimeout  = 90 * time.Second
	asyncImageSubmitCircuitDuration = 45 * time.Second
	asyncImageSubmitMaxAttempts     = 10
)

var asyncImageSubmitWorkerOnce sync.Once

var asyncImageSubmitCircuits = struct {
	sync.Mutex
	until map[int]time.Time
}{
	until: make(map[int]time.Time),
}

func StartAsyncImageSubmitWorker() {
	asyncImageSubmitWorkerOnce.Do(func() {
		workerID := asyncImageSubmitWorkerID()
		go asyncImageSubmitWorkerLoop(context.Background(), workerID)
		common.SysLog("async image submit worker started: " + workerID)
	})
}

func asyncImageSubmitWorkerID() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "node"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func asyncImageSubmitWorkerLoop(ctx context.Context, workerID string) {
	ticker := time.NewTicker(asyncImageSubmitInterval)
	defer ticker.Stop()

	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogError(ctx, fmt.Sprintf("async image submit worker panic: %v", r))
				}
			}()
			runAsyncImageSubmitWorkerOnce(ctx, workerID)
		}()

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func runAsyncImageSubmitWorkerOnce(ctx context.Context, workerID string) {
	now := time.Now()
	if err := model.ResetExpiredSubmittingAsyncImageTasks(now.Add(-asyncImageSubmitClaimTimeout).Unix(), asyncImageSubmitBatchSize); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("reset expired async image submitting tasks failed: %s", err.Error()))
	}

	tasks := model.GetQueuedAsyncImageTasks(asyncImageSubmitBatchSize, now.Unix())
	if len(tasks) == 0 {
		return
	}

	groups := groupAsyncImageSubmitTasks(tasks)
	for key, groupTasks := range groups {
		modelName := key.modelName
		candidates, err := model.GetEnabledChannelsForGroupModel(key.group, modelName)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("load async image submit channels failed group=%s model=%s: %s", key.group, modelName, err.Error()))
			requeueAsyncImageSubmitTasks(ctx, workerID, groupTasks, "load channels failed: "+err.Error())
			continue
		}
		scheduler := newAsyncImageSubmitGroupScheduler(candidates, modelName)
		for _, task := range groupTasks {
			processQueuedAsyncImageSubmitTask(ctx, workerID, task, modelName, scheduler)
		}
	}
}

type asyncImageSubmitTaskGroupKey struct {
	group     string
	modelName string
}

func groupAsyncImageSubmitTasks(tasks []*model.Task) map[asyncImageSubmitTaskGroupKey][]*model.Task {
	groups := make(map[asyncImageSubmitTaskGroupKey][]*model.Task)
	for _, task := range tasks {
		if task == nil {
			continue
		}
		modelName := asyncImageSubmitTaskModelName(task)
		if strings.TrimSpace(task.Group) == "" || strings.TrimSpace(modelName) == "" {
			continue
		}
		key := asyncImageSubmitTaskGroupKey{group: task.Group, modelName: modelName}
		groups[key] = append(groups[key], task)
	}
	return groups
}

func asyncImageSubmitTaskModelName(task *model.Task) string {
	modelName := firstNonEmptyString(task.Properties.OriginModelName, task.Properties.UpstreamModelName)
	if strings.TrimSpace(modelName) != "" {
		return strings.TrimSpace(modelName)
	}
	req, err := taskOriginalRequest(task)
	if err == nil && strings.TrimSpace(req.Model) != "" {
		return strings.TrimSpace(req.Model)
	}
	return ""
}

func processQueuedAsyncImageSubmitTask(ctx context.Context, workerID string, task *model.Task, modelName string, scheduler *asyncImageSubmitGroupScheduler) {
	now := time.Now()
	won, err := model.ClaimTaskForSubmit(task.ID, workerID, now.Unix())
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("claim async image task %s failed: %s", task.TaskID, err.Error()))
		return
	}
	if !won {
		return
	}

	if err = model.DB.First(task, task.ID).Error; err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("reload claimed async image task %d failed: %s", task.ID, err.Error()))
		return
	}

	ch := scheduler.Next(time.Now())
	if ch == nil {
		requeueOrFailAsyncImageSubmitTask(ctx, task, "no available async image channel")
		return
	}

	req, err := taskOriginalRequest(task)
	if err != nil {
		failAsyncImageSubmitTask(ctx, task, "invalid original request: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = modelName
	}

	submitCtx, cancelSubmit := asyncImageSubmitContext(ctx, ch.Type)
	defer cancelSubmit()
	result, info, submitErr := resubmitImageToChannel(submitCtx, task, req, modelName, ch)
	if submitErr != nil {
		if isFireflySubmitDeadlineExceeded(submitCtx, ch.Type) {
			reason := fmt.Sprintf("channel #%d submit timed out after %s", ch.Id, asyncImageFireflySubmitTimeout)
			logger.LogWarn(ctx, fmt.Sprintf("async image task %s failed permanently: %s", task.TaskID, reason))
			failAsyncImageSubmitTask(ctx, task, reason)
			return
		}
		if ch.Type == constant.ChannelTypeFirefly && isFireflyLocalStorageFailure(submitErr) {
			reason := fmt.Sprintf("channel #%d submit failed: %s", ch.Id, submitErr.Error())
			logger.LogWarn(ctx, fmt.Sprintf("async image task %s failed permanently: %s", task.TaskID, reason))
			failAsyncImageSubmitTask(ctx, task, reason)
			return
		}
		if shouldFailAsyncImageSubmitImmediately(submitErr) {
			logger.LogWarn(ctx, fmt.Sprintf("async image task %s submit to channel #%d failed permanently: %s", task.TaskID, ch.Id, submitErr.Error()))
			failAsyncImageSubmitTask(ctx, task, fmt.Sprintf("channel #%d submit failed: %s", ch.Id, submitErr.Error()))
			return
		}
		if shouldCircuitBreakAsyncImageSubmitError(submitErr) {
			openAsyncImageSubmitCircuit(ch.Id, submitErr)
		}
		logger.LogWarn(ctx, fmt.Sprintf("async image task %s submit to channel #%d failed: %s", task.TaskID, ch.Id, submitErr.Error()))
		requeueOrFailAsyncImageSubmitTask(ctx, task, fmt.Sprintf("channel #%d submit failed: %s", ch.Id, submitErr.Error()))
		return
	}

	tried := taskTriedChannelSet(task)
	tried[ch.Id] = struct{}{}
	fromStatus := task.Status
	reconcileAsyncImageSubmitBilling(ctx, task, result, info)
	applyAsyncImageSubmitResult(task, result, info, ch, tried)
	won, err = task.UpdateWithStatus(fromStatus)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("update async image task %s submit result failed: %s", task.TaskID, err.Error()))
		return
	}
	if !won {
		return
	}
	if task.Quota != 0 {
		model.UpdateChannelUsedQuota(ch.Id, task.Quota)
	}
	RunTaskAfterInsert(result, task)
	logger.LogInfo(ctx, fmt.Sprintf("async image task %s submitted to channel #%d", task.TaskID, ch.Id))
}

func reconcileAsyncImageSubmitBilling(ctx context.Context, task *model.Task, result *TaskSubmitResult, info *relaycommon.RelayInfo) {
	if task == nil || result == nil || info == nil {
		return
	}
	task.Properties.OriginModelName = info.OriginModelName
	task.Properties.UpstreamModelName = info.UpstreamModelName
	task.PrivateData.BillingContext = taskBillingContext(info)
	if result.Quota == task.Quota {
		return
	}
	if result.Quota == 0 {
		service.RefundTaskQuota(ctx, task, "actual image channel is free")
		task.Quota = 0
		return
	}
	service.RecalculateTaskQuota(ctx, task, result.Quota, "actual image channel pricing")
}

func requeueAsyncImageSubmitTasks(ctx context.Context, workerID string, tasks []*model.Task, reason string) {
	for _, task := range tasks {
		now := time.Now().Unix()
		won, err := model.ClaimTaskForSubmit(task.ID, workerID, now)
		if err != nil || !won {
			continue
		}
		if err = model.DB.First(task, task.ID).Error; err != nil {
			continue
		}
		requeueOrFailAsyncImageSubmitTask(ctx, task, reason)
	}
}

func applyAsyncImageSubmitResult(task *model.Task, result *TaskSubmitResult, relayInfo *relaycommon.RelayInfo, ch *model.Channel, tried map[int]struct{}) {
	task.ChannelId = ch.Id
	task.Platform = result.Platform
	task.Status = model.TaskStatusSubmitted
	task.Progress = taskcommon.ProgressSubmitted
	task.StartTime = 0
	task.FinishTime = 0
	task.FailReason = ""
	if relayInfo != nil {
		task.Action = relayInfo.Action
		task.Properties.OriginModelName = relayInfo.OriginModelName
		task.Properties.UpstreamModelName = relayInfo.UpstreamModelName
		task.PrivateData.Key = relayInfo.ApiKey
	}
	task.Data = result.TaskData
	task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
	task.PrivateData.ResultURL = ""
	task.PrivateData.ImageProtocol = result.ImageProtocol
	task.PrivateData.TriedChannelIDs = taskTriedChannelList(tried)
	task.PrivateData.LastFailureReason = ""
	task.SubmitState = model.TaskSubmitStateSubmitted
	task.SubmitAfter = 0
	task.SubmitClaimedAt = 0
	task.SubmitClaimedBy = ""
	if result.InitialTaskInfo != nil {
		ApplyTaskInfoToTask(task, result.InitialTaskInfo, result.TaskData)
	}
	clearTerminalFireflyTaskKey(task, ch.Type)
}

func asyncImageSubmitContext(parent context.Context, channelType int) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if channelType != constant.ChannelTypeFirefly {
		return parent, func() {}
	}
	return context.WithTimeout(parent, asyncImageFireflySubmitTimeout)
}

func isFireflySubmitDeadlineExceeded(ctx context.Context, channelType int) bool {
	return channelType == constant.ChannelTypeFirefly && ctx != nil && ctx.Err() == context.DeadlineExceeded
}

func isFireflyLocalStorageFailure(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "save result image failed") ||
		strings.Contains(text, "object storage returned no image url")
}

func requeueOrFailAsyncImageSubmitTask(ctx context.Context, task *model.Task, reason string) {
	task.SubmitAttempts++
	task.PrivateData.LastFailureReason = strings.TrimSpace(reason)
	if task.SubmitAttempts >= asyncImageSubmitMaxAttempts {
		failAsyncImageSubmitTask(ctx, task, reason)
		return
	}

	now := time.Now().Unix()
	fromStatus := task.Status
	task.Status = model.TaskStatusQueued
	task.Progress = "0%"
	task.SubmitState = model.TaskSubmitStateQueued
	task.SubmitAfter = now + int64(asyncImageSubmitBackoff(task.SubmitAttempts).Seconds())
	task.SubmitClaimedAt = 0
	task.SubmitClaimedBy = ""
	if won, err := task.UpdateWithStatus(fromStatus); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("requeue async image task %s failed: %s", task.TaskID, err.Error()))
	} else if won {
		logger.LogInfo(ctx, fmt.Sprintf("async image task %s requeued: %s", task.TaskID, reason))
	}
}

func failAsyncImageSubmitTask(ctx context.Context, task *model.Task, reason string) {
	now := time.Now().Unix()
	fromStatus := task.Status
	task.Status = model.TaskStatusFailure
	task.Progress = taskcommon.ProgressComplete
	task.FinishTime = now
	task.FailReason = strings.TrimSpace(reason)
	task.SubmitState = model.TaskSubmitStateFailed
	task.SubmitAfter = 0
	task.SubmitClaimedAt = 0
	task.SubmitClaimedBy = ""
	task.PrivateData.LastFailureReason = task.FailReason
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("fail async image task %s failed: %s", task.TaskID, err.Error()))
		return
	}
	if won && task.Quota != 0 {
		service.RefundTaskQuota(ctx, task, task.FailReason)
	}
}

func asyncImageSubmitBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	seconds := int(math.Pow(2, float64(attempt-1)))
	if seconds > 60 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

type asyncImageSubmitGroupScheduler struct {
	channels  []*model.Channel
	modelName string
	positions map[int64]int
}

type asyncImageWeightedSlot struct {
	channel   *model.Channel
	weight    int
	quota     int
	remainder float64
}

func newAsyncImageSubmitGroupScheduler(channels []*model.Channel, modelName string) *asyncImageSubmitGroupScheduler {
	return &asyncImageSubmitGroupScheduler{
		channels:  filterAsyncImageSubmitChannels(channels, modelName),
		modelName: modelName,
		positions: make(map[int64]int),
	}
}

func (s *asyncImageSubmitGroupScheduler) Next(now time.Time) *model.Channel {
	if s == nil || len(s.channels) == 0 {
		return nil
	}
	for _, priority := range asyncImageSubmitPriorities(s.channels) {
		available := make([]*model.Channel, 0)
		for _, ch := range s.channels {
			if ch.GetPriority() != priority {
				continue
			}
			if isAsyncImageSubmitCircuitOpen(ch.Id, now) {
				continue
			}
			available = append(available, ch)
		}
		schedule := buildWeightedChannelSchedule(available, asyncImageSubmitWindowSize)
		if len(schedule) == 0 {
			continue
		}
		pos := s.positions[priority] % len(schedule)
		s.positions[priority] = pos + 1
		return schedule[pos]
	}
	return nil
}

func filterAsyncImageSubmitChannels(channels []*model.Channel, modelName string) []*model.Channel {
	result := make([]*model.Channel, 0, len(channels))
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		if ch.Type == constant.ChannelTypeMihuifang || ch.Type == constant.ChannelTypeFirefly || ch.Type == constant.ChannelTypeGemini {
			result = append(result, ch)
		}
	}
	return result
}

func asyncImageSubmitPriorities(channels []*model.Channel) []int64 {
	seen := make(map[int64]struct{})
	priorities := make([]int64, 0)
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		priority := ch.GetPriority()
		if _, ok := seen[priority]; ok {
			continue
		}
		seen[priority] = struct{}{}
		priorities = append(priorities, priority)
	}
	sort.Slice(priorities, func(i, j int) bool {
		return priorities[i] > priorities[j]
	})
	return priorities
}

func buildWeightedChannelSchedule(channels []*model.Channel, window int) []*model.Channel {
	if len(channels) == 0 || window <= 0 {
		return nil
	}
	if len(channels) == 1 {
		return repeatChannel(channels[0], window)
	}

	slots := make([]asyncImageWeightedSlot, 0, len(channels))
	totalWeight := 0
	positiveWeights := 0
	for _, ch := range channels {
		weight := ch.GetWeight()
		if weight < 0 {
			weight = 0
		}
		if weight > 0 {
			totalWeight += weight
			positiveWeights++
		}
		slots = append(slots, asyncImageWeightedSlot{channel: ch, weight: weight})
	}

	if totalWeight == 0 {
		schedule := make([]*model.Channel, 0, window)
		for len(schedule) < window {
			for _, ch := range channels {
				schedule = append(schedule, ch)
				if len(schedule) == window {
					break
				}
			}
		}
		return schedule
	}

	assigned := 0
	for i := range slots {
		if slots[i].weight == 0 {
			continue
		}
		exact := float64(slots[i].weight) / float64(totalWeight) * float64(window)
		slots[i].quota = int(math.Floor(exact))
		if slots[i].quota == 0 && positiveWeights <= window {
			slots[i].quota = 1
		}
		slots[i].remainder = exact - math.Floor(exact)
		assigned += slots[i].quota
	}

	for assigned > window {
		idx := largestReducibleQuotaIndex(slots)
		if idx < 0 {
			break
		}
		slots[idx].quota--
		assigned--
	}
	for assigned < window {
		idx := largestRemainderIndex(slots)
		if idx < 0 {
			break
		}
		slots[idx].quota++
		slots[idx].remainder = 0
		assigned++
	}

	sort.SliceStable(slots, func(i, j int) bool {
		if slots[i].quota == slots[j].quota {
			return slots[i].channel.Id < slots[j].channel.Id
		}
		return slots[i].quota > slots[j].quota
	})

	schedule := make([]*model.Channel, 0, window)
	for len(schedule) < window {
		added := false
		for i := range slots {
			if slots[i].quota <= 0 {
				continue
			}
			schedule = append(schedule, slots[i].channel)
			slots[i].quota--
			added = true
			if len(schedule) == window {
				break
			}
		}
		if !added {
			break
		}
	}
	return schedule
}

func repeatChannel(ch *model.Channel, count int) []*model.Channel {
	result := make([]*model.Channel, 0, count)
	for i := 0; i < count; i++ {
		result = append(result, ch)
	}
	return result
}

func largestReducibleQuotaIndex(slots []asyncImageWeightedSlot) int {
	idx := -1
	for i := range slots {
		if slots[i].quota <= 1 {
			continue
		}
		if idx < 0 || slots[i].quota > slots[idx].quota ||
			(slots[i].quota == slots[idx].quota && slots[i].remainder < slots[idx].remainder) {
			idx = i
		}
	}
	return idx
}

func largestRemainderIndex(slots []asyncImageWeightedSlot) int {
	idx := -1
	for i := range slots {
		if slots[i].weight <= 0 {
			continue
		}
		if idx < 0 || slots[i].remainder > slots[idx].remainder ||
			(slots[i].remainder == slots[idx].remainder && slots[i].weight > slots[idx].weight) {
			idx = i
		}
	}
	return idx
}

func isAsyncImageSubmitCircuitOpen(channelID int, now time.Time) bool {
	asyncImageSubmitCircuits.Lock()
	defer asyncImageSubmitCircuits.Unlock()
	until, ok := asyncImageSubmitCircuits.until[channelID]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(asyncImageSubmitCircuits.until, channelID)
		return false
	}
	return true
}

func openAsyncImageSubmitCircuit(channelID int, err error) {
	asyncImageSubmitCircuits.Lock()
	asyncImageSubmitCircuits.until[channelID] = time.Now().Add(asyncImageSubmitCircuitDuration)
	asyncImageSubmitCircuits.Unlock()
	common.SysLog(fmt.Sprintf("async image submit circuit opened for channel #%d: %s", channelID, err.Error()))
}

func shouldCircuitBreakAsyncImageSubmitError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "timeout") ||
		strings.Contains(text, "deadline") ||
		strings.Contains(text, "connection refused") ||
		strings.Contains(text, "connection reset") ||
		strings.Contains(text, "eof") ||
		strings.Contains(text, "too many requests") {
		return true
	}
	status := parseStatusCodeFromError(text)
	return status == http.StatusTooManyRequests || status/100 == 5
}

func shouldFailAsyncImageSubmitImmediately(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	nonRetryable := []string{
		"image or reference_image_urls is required",
		"prompt field is required",
		"model field is required",
		"model price is required",
		"unsupported quality",
		"unsupported size",
		"unsupported ratio",
		"does not support ratio",
		"supports at most",
		"unmarshal_model_mapping_failed",
		"model_mapping_contains_cycle",
	}
	for _, item := range nonRetryable {
		if strings.Contains(text, item) {
			return true
		}
	}
	status := parseStatusCodeFromError(text)
	return status >= http.StatusBadRequest && status < http.StatusInternalServerError && status != http.StatusTooManyRequests
}

func parseStatusCodeFromError(text string) int {
	idx := strings.Index(text, "status ")
	if idx < 0 {
		return 0
	}
	rest := strings.TrimSpace(text[idx+len("status "):])
	fields := strings.FieldsFunc(rest, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if len(fields) == 0 {
		return 0
	}
	status, _ := strconv.Atoi(fields[0])
	return status
}
