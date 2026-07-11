package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type contextTaskAdaptor struct {
	taskcommon.BaseBilling
	requestURL string
}

func (*contextTaskAdaptor) Init(*relaycommon.RelayInfo) {}

func (*contextTaskAdaptor) ValidateRequestAndSetAction(*gin.Context, *relaycommon.RelayInfo) *dto.TaskError {
	return nil
}

func (a *contextTaskAdaptor) BuildRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.requestURL, nil
}

func (*contextTaskAdaptor) BuildRequestHeader(*gin.Context, *http.Request, *relaycommon.RelayInfo) error {
	return nil
}

func (*contextTaskAdaptor) BuildRequestBody(*gin.Context, *relaycommon.RelayInfo) (io.Reader, error) {
	return strings.NewReader("{}"), nil
}

func (*contextTaskAdaptor) DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (*http.Response, error) {
	return nil, nil
}

func (*contextTaskAdaptor) DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	return "", nil, nil
}

func (*contextTaskAdaptor) GetModelList() []string { return nil }
func (*contextTaskAdaptor) GetChannelName() string { return "context-test" }

func (*contextTaskAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return nil, nil
}

func (*contextTaskAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}

func TestDoTaskAPIRequestPropagatesCallerCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
	}))
	defer func() {
		close(releaseRequest)
		server.Close()
	}()

	service.InitHttpClient()
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/async/generations", strings.NewReader("{}"))
	c.Request = c.Request.WithContext(requestCtx)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	adaptor := &contextTaskAdaptor{requestURL: server.URL}

	result := make(chan error, 1)
	go func() {
		_, err := DoTaskApiRequest(adaptor, c, info, strings.NewReader("{}"))
		result <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("upstream request did not start")
	}
	cancelRequest()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("canceled task request returned no error")
		}
	case <-time.After(time.Second):
		t.Fatal("task request did not stop after caller cancellation")
	}
}

var _ TaskAdaptor = (*contextTaskAdaptor)(nil)
