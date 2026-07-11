package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestFireflyChannelUsesOpenAIAPIType(t *testing.T) {
	apiType, ok := ChannelType2APIType(constant.ChannelTypeFirefly)
	if !ok || apiType != constant.APITypeOpenAI {
		t.Fatalf("ChannelType2APIType(Firefly) = (%d, %v), want (%d, true)", apiType, ok, constant.APITypeOpenAI)
	}
}

func TestFireflyChannelOnlyAdvertisesImageEndpoints(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeFirefly, "gemini-3-pro-image")
	want := []constant.EndpointType{constant.EndpointTypeImageGeneration, constant.EndpointTypeAsyncImageGeneration}
	if len(got) != len(want) {
		t.Fatalf("endpoint types = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("endpoint types = %#v, want %#v", got, want)
		}
	}
}
