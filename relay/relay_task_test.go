package relay

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
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
