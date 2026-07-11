package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

func TestFireflyModelsRegisteredForChannelEditor(t *testing.T) {
	wantModels := []string{"gpt-image-2", "nanobanana", "nanobanana2", "nanobananapro"}
	channelModels := channelId2Models[constant.ChannelTypeFirefly]
	for _, modelName := range wantModels {
		if !common.StringsContains(channelModels, modelName) {
			t.Fatalf("Firefly channel models missing %q: %#v", modelName, channelModels)
		}
		registered, ok := openAIModelsMap[modelName]
		if !ok {
			t.Fatalf("channel editor model pool missing %q", modelName)
		}
		if registered.OwnedBy != "mihuifang" {
			t.Fatalf("model %q owned_by = %q, want mihuifang", modelName, registered.OwnedBy)
		}
	}
}
