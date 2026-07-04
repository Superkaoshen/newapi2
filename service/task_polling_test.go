package service

import (
	"context"
	"testing"

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
