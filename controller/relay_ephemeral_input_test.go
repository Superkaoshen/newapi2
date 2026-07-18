package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func TestSetTaskOriginalRequestSkipsEmbeddedImage(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "nanobananapro",
		Prompt: "edit",
		Image:  "data:image/png;base64,aGVsbG8=",
	})
	task := &model.Task{}

	setTaskOriginalRequest(c, task)

	if !task.PrivateData.EphemeralInput {
		t.Fatal("embedded image task must be marked ephemeral")
	}
	if task.PrivateData.OriginalRequest != "" {
		t.Fatalf("embedded image was persisted: %q", task.PrivateData.OriginalRequest)
	}
}

func TestSetTaskOriginalRequestPersistsRemoteImageURL(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "nanobananapro",
		Prompt: "edit",
		Image:  "https://cdn.example.com/input.png",
	})
	task := &model.Task{}

	setTaskOriginalRequest(c, task)

	if task.PrivateData.EphemeralInput {
		t.Fatal("remote image URL must remain eligible for durable retries")
	}
	if !strings.Contains(task.PrivateData.OriginalRequest, "https://cdn.example.com/input.png") {
		t.Fatalf("remote image request was not persisted: %q", task.PrivateData.OriginalRequest)
	}
}

func TestSetTaskOriginalRequestTreatsMultipartAsEphemeral(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{Header: http.Header{"Content-Type": []string{"multipart/form-data; boundary=test"}}}
	c.Set("task_request", relaycommon.TaskSubmitReq{Model: "nanobananapro", Prompt: "edit"})
	task := &model.Task{}

	setTaskOriginalRequest(c, task)

	if !task.PrivateData.EphemeralInput || task.PrivateData.OriginalRequest != "" {
		t.Fatalf("multipart task persistence = %#v, want ephemeral without original request", task.PrivateData)
	}
}
