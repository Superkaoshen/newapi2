package relay

import (
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

func TestTaskModel2DtoHidesUpstreamModel(t *testing.T) {
	task := taskWithMappedModel()
	task.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeFirefly))
	task.FailReason = "firefly-nano-banana-pro-2k-16x9 failed"
	task.PrivateData.ResultURL = "https://internal.invalid/nanobananapro/result.png"
	task.Data = []byte(`{"model":"gpt-image-2","reason":"firely-nano-banana-pro-2k-16x9 failed"}`)

	got := TaskModel2Dto(task)
	properties, ok := got.Properties.(model.Properties)
	if !ok {
		t.Fatalf("properties type = %T, want model.Properties", got.Properties)
	}
	if properties.OriginModelName != "gemini-3-pro-image" {
		t.Fatalf("origin model = %q, want public request model", properties.OriginModelName)
	}
	if properties.UpstreamModelName != "" {
		t.Fatalf("upstream model = %q, want empty", properties.UpstreamModelName)
	}
	if task.Properties.UpstreamModelName != "firefly-nano-banana-pro-2k-16x9" {
		t.Fatal("public DTO conversion mutated the stored task properties")
	}

	body, err := common.Marshal(got)
	if err != nil {
		t.Fatalf("marshal public task DTO: %v", err)
	}
	serialized := string(body)
	for _, forbidden := range []string{"upstream_model_name", "firefly-nano-banana", "firely-nano-banana", "nanobananapro", "gpt-image-2"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("public task DTO leaked %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, "gemini-3-pro-image") {
		t.Fatalf("public task DTO leaked upstream model: %s", serialized)
	}
}

func TestTaskModel2AdminDtoKeepsUpstreamModel(t *testing.T) {
	got := TaskModel2AdminDto(taskWithMappedModel())
	properties, ok := got.Properties.(model.Properties)
	if !ok {
		t.Fatalf("properties type = %T, want model.Properties", got.Properties)
	}
	if properties.UpstreamModelName != "firefly-nano-banana-pro-2k-16x9" {
		t.Fatalf("upstream model = %q, want admin-visible actual model", properties.UpstreamModelName)
	}

	body, err := common.Marshal(got)
	if err != nil {
		t.Fatalf("marshal admin task DTO: %v", err)
	}
	serialized := string(body)
	if !strings.Contains(serialized, `"upstream_model_name":"firefly-nano-banana-pro-2k-16x9"`) {
		t.Fatalf("admin task DTO missing upstream model: %s", serialized)
	}
}

func taskWithMappedModel() *model.Task {
	return &model.Task{
		TaskID: "task_public",
		Properties: model.Properties{
			Input:             "draw a cat",
			OriginModelName:   "gemini-3-pro-image",
			UpstreamModelName: "firefly-nano-banana-pro-2k-16x9",
		},
	}
}
