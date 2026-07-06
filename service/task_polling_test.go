package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestCollectTaskForPollingSkipsMissingChannel(t *testing.T) {
	task := &model.Task{
		ID:        1,
		TaskID:    "task_public",
		ChannelId: 0,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-task",
		},
	}
	taskChannelM := make(map[int][]string)
	taskM := make(map[string]*model.Task)
	nullTaskIds := make([]int64, 0)

	collectTaskForPolling(context.Background(), task, taskChannelM, taskM, &nullTaskIds)

	require.Empty(t, taskChannelM)
	require.Empty(t, taskM)
	require.Empty(t, nullTaskIds)
}

func TestLocalGeminiImageTaskSkipsPollingWithMappedNanoBananaModel(t *testing.T) {
	task := &model.Task{
		ID:        1,
		TaskID:    "task_public",
		ChannelId: 6,
		Platform:  constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeGemini)),
		Properties: model.Properties{
			OriginModelName:   "gemini-3-pro-image",
			UpstreamModelName: "nano-banana-pro",
		},
		PrivateData: model.TaskPrivateData{
			RequestBody: "encoded-request-body",
		},
	}

	require.True(t, isLocalGeminiImageTask(task))
}
