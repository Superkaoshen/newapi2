package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestInitTaskDoesNotPersistFireflyAPIKey(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeFirefly,
			ApiKey:      "service-secret",
		},
	}

	task := InitTask(constant.TaskPlatform("59"), info)
	if task.PrivateData.Key != "" {
		t.Fatalf("Firefly task persisted API key %q", task.PrivateData.Key)
	}
}

func TestInitTaskStillPersistsPollingChannelAPIKey(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeMihuifang,
			ApiKey:      "polling-secret",
		},
	}

	task := InitTask(constant.TaskPlatform("58"), info)
	if task.PrivateData.Key != "polling-secret" {
		t.Fatalf("polling task API key = %q, want retained key", task.PrivateData.Key)
	}
}
