package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

func TestIsHTTPSuccessStatusAcceptsAny2xx(t *testing.T) {
	for _, statusCode := range []int{http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent} {
		if !isHTTPSuccessStatus(statusCode) {
			t.Fatalf("isHTTPSuccessStatus(%d) = false, want true", statusCode)
		}
	}
	for _, statusCode := range []int{http.StatusContinue, http.StatusMultipleChoices, http.StatusBadRequest, http.StatusInternalServerError} {
		if isHTTPSuccessStatus(statusCode) {
			t.Fatalf("isHTTPSuccessStatus(%d) = true, want false", statusCode)
		}
	}
}

func TestCanQueueAsyncImageTaskKeepsEmbeddedImagesRequestScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		contentType string
		body        string
		want        bool
	}{
		{
			name:        "remote URL remains durable",
			contentType: "application/json",
			body:        `{"model":"nanobananapro","prompt":"edit","image":"https://cdn.example.com/input.png"}`,
			want:        true,
		},
		{
			name:        "data URI submits directly",
			contentType: "application/json",
			body:        `{"model":"nanobananapro","prompt":"edit","image":"data:image/png;base64,aGVsbG8="}`,
			want:        false,
		},
		{
			name:        "raw base64 submits directly",
			contentType: "application/json",
			body:        `{"model":"nanobananapro","prompt":"edit","images":["aGVsbG8="]}`,
			want:        false,
		},
		{
			name:        "multipart remains request scoped",
			contentType: "multipart/form-data; boundary=test",
			body:        "",
			want:        false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/async/generations", strings.NewReader(test.body))
			c.Request.Header.Set("Content-Type", test.contentType)
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeFirefly)
			defer common.CleanupBodyStorage(c)

			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAsyncImageSubmit}
			if got := CanQueueAsyncImageTask(c, info); got != test.want {
				t.Fatalf("CanQueueAsyncImageTask() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestBuildWeightedChannelScheduleUsesWeightWindow(t *testing.T) {
	high := testAsyncImageChannel(1, constant.ChannelTypeMihuifang, 0, 9)
	low := testAsyncImageChannel(2, constant.ChannelTypeGemini, 0, 1)

	schedule := buildWeightedChannelSchedule([]*model.Channel{high, low}, 10)
	if len(schedule) != 10 {
		t.Fatalf("schedule length = %d, want 10", len(schedule))
	}

	counts := countScheduledChannels(schedule)
	if counts[high.Id] != 9 || counts[low.Id] != 1 {
		t.Fatalf("counts = %#v, want high=9 low=1", counts)
	}
}

func TestBuildWeightedChannelScheduleAllZeroWeightsRoundRobin(t *testing.T) {
	ch1 := testAsyncImageChannel(1, constant.ChannelTypeMihuifang, 0, 0)
	ch2 := testAsyncImageChannel(2, constant.ChannelTypeGemini, 0, 0)
	ch3 := testAsyncImageChannel(3, constant.ChannelTypeMihuifang, 0, 0)

	schedule := buildWeightedChannelSchedule([]*model.Channel{ch1, ch2, ch3}, 10)
	if len(schedule) != 10 {
		t.Fatalf("schedule length = %d, want 10", len(schedule))
	}

	counts := countScheduledChannels(schedule)
	if counts[ch1.Id] != 4 || counts[ch2.Id] != 3 || counts[ch3.Id] != 3 {
		t.Fatalf("counts = %#v, want 4/3/3", counts)
	}
}

func TestAsyncImageSubmitSchedulerFallsBackWhenPriorityCircuitOpen(t *testing.T) {
	asyncImageSubmitCircuits.Lock()
	asyncImageSubmitCircuits.until = make(map[int]time.Time)
	asyncImageSubmitCircuits.Unlock()

	highA := testAsyncImageChannel(1, constant.ChannelTypeMihuifang, 10, 1)
	highB := testAsyncImageChannel(2, constant.ChannelTypeGemini, 10, 1)
	low := testAsyncImageChannel(3, constant.ChannelTypeMihuifang, 1, 1)
	openAsyncImageSubmitCircuit(highA.Id, errTestAsyncImageSubmitCircuit{})
	openAsyncImageSubmitCircuit(highB.Id, errTestAsyncImageSubmitCircuit{})

	scheduler := newAsyncImageSubmitGroupScheduler([]*model.Channel{highA, highB, low}, "test-image-model")
	ch := scheduler.Next(time.Now())
	if ch == nil || ch.Id != low.Id {
		t.Fatalf("scheduler returned channel %#v, want low priority channel %d", ch, low.Id)
	}
}

func TestShouldFailAsyncImageSubmitImmediately(t *testing.T) {
	if !shouldFailAsyncImageSubmitImmediately(fmt.Errorf("image or reference_image_urls is required")) {
		t.Fatal("imageone validation error should fail immediately")
	}
	if !shouldFailAsyncImageSubmitImmediately(fmt.Errorf("status 400: invalid request")) {
		t.Fatal("4xx validation error should fail immediately")
	}
	if shouldFailAsyncImageSubmitImmediately(fmt.Errorf("status 429: rate limited")) {
		t.Fatal("429 should remain retryable")
	}
	if shouldFailAsyncImageSubmitImmediately(fmt.Errorf("status 500: upstream error")) {
		t.Fatal("5xx should remain retryable")
	}
}

func TestFilterAsyncImageSubmitChannelsKeepsGeminiNanoBananaAlias(t *testing.T) {
	mapping := `{"gemini-3-pro-image":"nano-banana-pro"}`
	gemini := testAsyncImageChannel(2, constant.ChannelTypeGemini, 0, 1)
	gemini.ModelMapping = &mapping

	channels := filterAsyncImageSubmitChannels([]*model.Channel{gemini}, "gemini-3-pro-image")
	if len(channels) != 1 || channels[0].Id != gemini.Id {
		t.Fatalf("channels = %#v, want gemini channel %d", channelIDs(channels), gemini.Id)
	}
}

func TestFilterAsyncImageSubmitChannelsDoesNotInferProviderFromMappedModelName(t *testing.T) {
	mapping := `{"gemini-3-pro-image":"nanobanana-7"}`
	mihuifang := testAsyncImageChannel(1, constant.ChannelTypeMihuifang, 0, 1)
	mihuifang.ModelMapping = &mapping
	gemini := testAsyncImageChannel(2, constant.ChannelTypeGemini, 0, 1)
	gemini.ModelMapping = &mapping

	channels := filterAsyncImageSubmitChannels([]*model.Channel{mihuifang, gemini}, "gemini-3-pro-image")
	if len(channels) != 2 {
		t.Fatalf("channels = %#v, want both configured async image channels", channelIDs(channels))
	}
}

func TestFilterAsyncImageSubmitChannelsKeepsFirefly(t *testing.T) {
	mapping := `{"gemini-3-pro-image":"nanobananapro"}`
	firefly := testAsyncImageChannel(3, constant.ChannelTypeFirefly, 0, 1)
	firefly.ModelMapping = &mapping

	channels := filterAsyncImageSubmitChannels([]*model.Channel{firefly}, "gemini-3-pro-image")
	if len(channels) != 1 || channels[0].Id != firefly.Id {
		t.Fatalf("channels = %#v, want Firefly channel %d", channelIDs(channels), firefly.Id)
	}
}

func TestFilterAsyncImageSubmitChannelsKeepsGeminiImageModels(t *testing.T) {
	geminiMapping := `{"public-image":"gemini-3-pro-image-preview"}`
	gemini := testAsyncImageChannel(1, constant.ChannelTypeGemini, 0, 1)
	gemini.ModelMapping = &geminiMapping
	imagenMapping := `{"public-imagen":"imagen-4.0-generate-001"}`
	imagen := testAsyncImageChannel(2, constant.ChannelTypeGemini, 0, 1)
	imagen.ModelMapping = &imagenMapping

	geminiChannels := filterAsyncImageSubmitChannels([]*model.Channel{gemini}, "public-image")
	if len(geminiChannels) != 1 || geminiChannels[0].Id != gemini.Id {
		t.Fatalf("gemini channels = %#v, want channel %d", channelIDs(geminiChannels), gemini.Id)
	}
	imagenChannels := filterAsyncImageSubmitChannels([]*model.Channel{imagen}, "public-imagen")
	if len(imagenChannels) != 1 || imagenChannels[0].Id != imagen.Id {
		t.Fatalf("imagen channels = %#v, want channel %d", channelIDs(imagenChannels), imagen.Id)
	}
}

type errTestAsyncImageSubmitCircuit struct{}

func (errTestAsyncImageSubmitCircuit) Error() string {
	return "status 429: rate limited"
}

func testAsyncImageChannel(id int, channelType int, priority int64, weight uint) *model.Channel {
	return &model.Channel{
		Id:       id,
		Type:     channelType,
		Priority: &priority,
		Weight:   &weight,
	}
}

func countScheduledChannels(schedule []*model.Channel) map[int]int {
	counts := make(map[int]int)
	for _, ch := range schedule {
		counts[ch.Id]++
	}
	return counts
}

func channelIDs(channels []*model.Channel) []int {
	ids := make([]int, 0, len(channels))
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		ids = append(ids, ch.Id)
	}
	return ids
}
