package common

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestTaskSubmitReqHasEmbeddedImage(t *testing.T) {
	tests := []struct {
		name string
		req  TaskSubmitReq
		want bool
	}{
		{name: "empty", req: TaskSubmitReq{}, want: false},
		{name: "remote image", req: TaskSubmitReq{Image: "https://cdn.example.com/input.png"}, want: false},
		{name: "remote image list", req: TaskSubmitReq{Images: []string{"http://cdn.example.com/a.png", "https://cdn.example.com/b.png"}}, want: false},
		{name: "data URI", req: TaskSubmitReq{Image: "data:image/png;base64,aGVsbG8="}, want: true},
		{name: "raw base64", req: TaskSubmitReq{Image: "aGVsbG8="}, want: true},
		{name: "relative path", req: TaskSubmitReq{ReferenceImages: []string{"/private/input.png"}}, want: true},
		{name: "mask data URI", req: TaskSubmitReq{Mask: []byte(`"data:image/png;base64,aGVsbG8="`)}, want: true},
		{name: "remote mask", req: TaskSubmitReq{Mask: []byte(`"https://cdn.example.com/mask.png"`)}, want: false},
		{name: "invalid mask shape is ephemeral", req: TaskSubmitReq{Mask: []byte(`{"data":"aGVsbG8="}`)}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.req.HasEmbeddedImage())
		})
	}
}
