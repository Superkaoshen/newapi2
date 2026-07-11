package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestTaskToDtoUsesRoleAppropriateModelProperties(t *testing.T) {
	task := &model.Task{
		Properties: model.Properties{
			OriginModelName:   "gemini-3-pro-image",
			UpstreamModelName: "firefly-nano-banana-pro-2k-16x9",
		},
	}

	userDTO := taskToDto(task, false)
	userProperties, ok := userDTO.Properties.(model.Properties)
	if !ok {
		t.Fatalf("user properties type = %T, want model.Properties", userDTO.Properties)
	}
	if userProperties.UpstreamModelName != "" {
		t.Fatalf("user upstream model = %q, want empty", userProperties.UpstreamModelName)
	}

	adminDTO := taskToDto(task, true)
	adminProperties, ok := adminDTO.Properties.(model.Properties)
	if !ok {
		t.Fatalf("admin properties type = %T, want model.Properties", adminDTO.Properties)
	}
	if adminProperties.UpstreamModelName != "firefly-nano-banana-pro-2k-16x9" {
		t.Fatalf("admin upstream model = %q, want actual model", adminProperties.UpstreamModelName)
	}
}
