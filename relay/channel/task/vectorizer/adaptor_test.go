package vectorizer

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func TestAddTaskResponseAcceptsNumericID(t *testing.T) {
	var result addTaskResponse
	err := common.Unmarshal([]byte(`{"code":0,"id":123456}`), &result)
	if err != nil {
		t.Fatalf("unmarshal add task response: %v", err)
	}

	if got := result.ID.String(); got != "123456" {
		t.Fatalf("ID = %q, want %q", got, "123456")
	}
}

func TestAddTaskResponseAcceptsStringID(t *testing.T) {
	var result addTaskResponse
	err := common.Unmarshal([]byte(`{"code":0,"id":"task-123"}`), &result)
	if err != nil {
		t.Fatalf("unmarshal add task response: %v", err)
	}

	if got := result.ID.String(); got != "task-123" {
		t.Fatalf("ID = %q, want %q", got, "task-123")
	}
}

func TestParseTaskResultAcceptsNumericTaskID(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info, err := adaptor.ParseTaskResult([]byte(`{"code":0,"taskid":123456,"url":"https://example.com/out.svg"}`))
	if err != nil {
		t.Fatalf("parse task result: %v", err)
	}

	if info.Status != model.TaskStatusSuccess {
		t.Fatalf("Status = %q, want %q", info.Status, model.TaskStatusSuccess)
	}
	if info.TaskID != "123456" {
		t.Fatalf("TaskID = %q, want %q", info.TaskID, "123456")
	}
}
