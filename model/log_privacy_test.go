package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestFormatUserLogsRemovesModelMappingFields(t *testing.T) {
	logs := []*Log{
		{
			Id:          99,
			ModelName:   "gemini-3-pro-image",
			ChannelName: "internal-channel",
			Other: `{
				"is_model_mapped": true,
				"upstream_model_name": "firefly-nano-banana-pro-2k-16x9",
				"model_price": 0.25,
				"admin_info": {"use_channel": [1]},
				"stream_status": {"status": "ok"}
			}`,
		},
	}

	formatUserLogs(logs, 7)

	if logs[0].Id != 8 {
		t.Fatalf("log id = %d, want 8", logs[0].Id)
	}
	if logs[0].ChannelName != "" {
		t.Fatalf("channel name = %q, want empty", logs[0].ChannelName)
	}
	if logs[0].ModelName != "gemini-3-pro-image" {
		t.Fatalf("model name = %q, want public request model", logs[0].ModelName)
	}

	other, err := common.StrToMap(logs[0].Other)
	if err != nil {
		t.Fatalf("parse formatted log other: %v", err)
	}
	for _, key := range []string{"admin_info", "stream_status", "is_model_mapped", "upstream_model_name"} {
		if _, exists := other[key]; exists {
			t.Fatalf("formatted user log still contains %q: %s", key, logs[0].Other)
		}
	}
	if got := other["model_price"]; got != 0.25 {
		t.Fatalf("model_price = %#v, want 0.25", got)
	}
}
