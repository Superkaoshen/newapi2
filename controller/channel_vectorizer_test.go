package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

func TestValidateChannelAllowsVectorizerWithoutKey(t *testing.T) {
	channel := &model.Channel{
		Type:   constant.ChannelTypeVectorizer,
		Key:    "",
		Models: "vectorizer",
		Group:  "default",
	}

	if err := validateChannel(channel, true); err != nil {
		t.Fatalf("validateChannel() error = %v", err)
	}
}

func TestValidateChannelRequiresKeyForOtherChannels(t *testing.T) {
	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "",
		Models: "gpt-4o",
		Group:  "default",
	}

	if err := validateChannel(channel, true); err == nil {
		t.Fatal("validateChannel() error = nil, want non-nil")
	}
}
