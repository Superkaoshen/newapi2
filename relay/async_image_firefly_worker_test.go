package relay

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

func TestFireflyDoesNotUseGenericSyncImageFallback(t *testing.T) {
	if supportsSyncImageFailoverChannel(constant.ChannelTypeFirefly, "nanobananapro") {
		t.Fatal("Firefly must not fall back to the generic /v1/images/generations adaptor")
	}
	if !supportsAsyncImageFailoverChannel(constant.ChannelTypeFirefly) {
		t.Fatal("Firefly task adaptor must remain enabled")
	}
}

func TestAsyncImageFireflySubmitContextExpiresBeforeClaim(t *testing.T) {
	if asyncImageFireflySubmitTimeout >= asyncImageSubmitClaimTimeout {
		t.Fatalf("Firefly submit timeout %s must be shorter than claim timeout %s", asyncImageFireflySubmitTimeout, asyncImageSubmitClaimTimeout)
	}

	startedAt := time.Now()
	ctx, cancel := asyncImageSubmitContext(context.Background(), constant.ChannelTypeFirefly)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("Firefly submit context has no deadline")
	}
	remaining := deadline.Sub(startedAt)
	if remaining <= 0 || remaining > asyncImageFireflySubmitTimeout {
		t.Fatalf("Firefly submit deadline remaining = %s, want (0, %s]", remaining, asyncImageFireflySubmitTimeout)
	}

	plainCtx, plainCancel := asyncImageSubmitContext(context.Background(), constant.ChannelTypeMihuifang)
	defer plainCancel()
	if _, ok := plainCtx.Deadline(); ok {
		t.Fatal("non-Firefly submit context unexpectedly has a worker deadline")
	}
}

func TestFireflySubmitDeadlineIsPermanentOnlyForFirefly(t *testing.T) {
	expiredCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	if !isFireflySubmitDeadlineExceeded(expiredCtx, constant.ChannelTypeFirefly) {
		t.Fatal("expired Firefly submit must be classified as permanent")
	}
	if isFireflySubmitDeadlineExceeded(expiredCtx, constant.ChannelTypeMihuifang) {
		t.Fatal("non-Firefly submit must retain its existing retry behavior")
	}
	if isFireflySubmitDeadlineExceeded(context.Background(), constant.ChannelTypeFirefly) {
		t.Fatal("active Firefly submit was incorrectly classified as timed out")
	}
}

func TestFireflyLocalStorageFailureIsPermanent(t *testing.T) {
	for _, message := range []string{
		"save result image failed",
		"object storage returned no image URL",
	} {
		if !isFireflyLocalStorageFailure(fmt.Errorf("%s", message)) {
			t.Fatalf("storage failure %q was not classified as permanent", message)
		}
	}
	if isFireflyLocalStorageFailure(fmt.Errorf("status 503")) {
		t.Fatal("upstream 503 should retain failover behavior")
	}
}

func TestApplyAsyncImageSubmitResultClearsTerminalFireflyKey(t *testing.T) {
	task := &model.Task{Status: model.TaskStatusQueued}
	result := &TaskSubmitResult{
		Platform: constant.TaskPlatform("59"),
		InitialTaskInfo: &relaycommon.TaskInfo{
			Status: model.TaskStatusSuccess,
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "public-image-model",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			Action: "generate",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "service-secret",
			UpstreamModelName: "nanobananapro",
		},
	}
	channel := &model.Channel{Id: 1, Type: constant.ChannelTypeFirefly}

	applyAsyncImageSubmitResult(task, result, info, channel, map[int]struct{}{channel.Id: {}})

	if task.Status != model.TaskStatusSuccess {
		t.Fatalf("task status = %s, want %s", task.Status, model.TaskStatusSuccess)
	}
	if task.PrivateData.Key != "" {
		t.Fatalf("terminal Firefly task persisted API key %q", task.PrivateData.Key)
	}
}

func TestReconcileAsyncImageSubmitBillingUsesActualChannelMetadata(t *testing.T) {
	task := &model.Task{
		Quota: 1200,
		Properties: model.Properties{
			OriginModelName:   "public-image-model",
			UpstreamModelName: "initial-upstream-model",
		},
	}
	result := &TaskSubmitResult{Quota: 1200}
	info := &relaycommon.RelayInfo{
		OriginModelName: "public-image-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "actual-upstream-model",
		},
		PriceData: types.PriceData{
			ModelPrice: 0.12,
			Quota:      1200,
			OtherRatios: map[string]float64{
				"price_tier": 1.5,
			},
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.2},
		},
	}

	reconcileAsyncImageSubmitBilling(context.Background(), task, result, info)

	if task.Properties.UpstreamModelName != "actual-upstream-model" {
		t.Fatalf("upstream model = %q, want actual channel mapping", task.Properties.UpstreamModelName)
	}
	if task.PrivateData.BillingContext == nil || task.PrivateData.BillingContext.ModelPrice != 0.12 {
		t.Fatalf("billing context = %#v, want actual channel price", task.PrivateData.BillingContext)
	}
	if got := task.PrivateData.BillingContext.OtherRatios["price_tier"]; got != 1.5 {
		t.Fatalf("price_tier ratio = %v, want 1.5", got)
	}
}
