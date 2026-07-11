package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func TestImageTaskOnlyChannelsSupportOnlyImageEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		method  string
		path    string
		allowed bool
	}{
		{name: "chat completions", method: http.MethodPost, path: "/v1/chat/completions", allowed: false},
		{name: "responses", method: http.MethodPost, path: "/v1/responses", allowed: false},
		{name: "image generation", method: http.MethodPost, path: "/v1/images/generations", allowed: true},
		{name: "image edit", method: http.MethodPost, path: "/v1/images/edits", allowed: true},
		{name: "async image submit", method: http.MethodPost, path: "/v1/async/generations", allowed: true},
		{name: "async image fetch", method: http.MethodGet, path: "/v1/tasks/task_public", allowed: true},
	}

	for _, channelType := range []int{constant.ChannelTypeMihuifang, constant.ChannelTypeFirefly} {
		for _, test := range tests {
			t.Run(constant.GetChannelTypeName(channelType)+"/"+test.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(test.method, test.path, nil)
				got := channelSupportsRequestEndpoint(c, testDistributorChannel(1, channelType))
				if got != test.allowed {
					t.Fatalf("channelSupportsRequestEndpoint(%s, %s) = %v, want %v", constant.GetChannelTypeName(channelType), test.path, got, test.allowed)
				}
			})
		}
	}
}

func TestDistributeFixedFireflyRejectsChatCompletions(t *testing.T) {
	firefly := testDistributorChannel(59, constant.ChannelTypeFirefly)
	deps := testDistributorDependencies()
	deps.getChannelByID = func(id int, _ bool) (*model.Channel, error) {
		if id != firefly.Id {
			t.Fatalf("fixed channel id = %d, want %d", id, firefly.Id)
		}
		return firefly, nil
	}

	status, selectedType, called, body := runDistributorRequest(t, http.MethodPost, "/v1/chat/completions", deps, firefly.Id)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", status, http.StatusBadRequest, body)
	}
	if called || selectedType == constant.ChannelTypeFirefly {
		t.Fatalf("fixed Firefly channel reached chat handler: called=%v type=%d", called, selectedType)
	}
}

func TestDistributeAffinityFireflyFallsBackForChatCompletions(t *testing.T) {
	firefly := testDistributorChannel(59, constant.ChannelTypeFirefly)
	openAI := testDistributorChannel(1, constant.ChannelTypeOpenAI)
	deps := testDistributorDependencies()
	deps.getPreferredChannelByAffinity = func(_ *gin.Context, modelName, group string) (int, bool) {
		if modelName != "gemini-3-pro-image" || group != "default" {
			t.Fatalf("affinity lookup = (%q, %q), want public model/default", modelName, group)
		}
		return firefly.Id, true
	}
	deps.getCachedChannel = func(id int) (*model.Channel, error) {
		if id != firefly.Id {
			t.Fatalf("cached affinity channel id = %d, want %d", id, firefly.Id)
		}
		return firefly, nil
	}
	deps.isChannelEnabledForGroupModel = func(string, string, int) bool {
		t.Fatal("endpoint-incompatible affinity channel must be rejected before group/model validation")
		return false
	}
	deps.getRandomSatisfiedChannel = func(param *service.RetryParam) (*model.Channel, string, error) {
		return openAI, param.TokenGroup, nil
	}

	status, selectedType, called, body := runDistributorRequest(t, http.MethodPost, "/v1/chat/completions", deps, 0)
	if status != http.StatusNoContent || !called {
		t.Fatalf("status=%d called=%v, want successful fallback; body=%s", status, called, body)
	}
	if selectedType != constant.ChannelTypeOpenAI {
		t.Fatalf("selected channel type = %d, want OpenAI", selectedType)
	}
}

func TestDistributeRandomSelectionSkipsImageOnlyChannelsForChatCompletions(t *testing.T) {
	firefly := testDistributorChannel(59, constant.ChannelTypeFirefly)
	mihuifang := testDistributorChannel(58, constant.ChannelTypeMihuifang)
	openAI := testDistributorChannel(1, constant.ChannelTypeOpenAI)
	deps := testDistributorDependencies()
	deps.getRandomSatisfiedChannel = func(param *service.RetryParam) (*model.Channel, string, error) {
		return firefly, param.TokenGroup, nil
	}
	deps.getEnabledChannelsForGroupModel = func(group, modelName string) ([]*model.Channel, error) {
		if group != "default" || modelName != "gemini-3-pro-image" {
			t.Fatalf("fallback lookup = (%q, %q), want default/public model", group, modelName)
		}
		return []*model.Channel{firefly, mihuifang, openAI}, nil
	}

	status, selectedType, called, body := runDistributorRequest(t, http.MethodPost, "/v1/chat/completions", deps, 0)
	if status != http.StatusNoContent || !called {
		t.Fatalf("status=%d called=%v, want successful compatible fallback; body=%s", status, called, body)
	}
	if selectedType != constant.ChannelTypeOpenAI {
		t.Fatalf("selected channel type = %d, want OpenAI", selectedType)
	}
}

func TestDistributeFixedFireflyAllowsImageGeneration(t *testing.T) {
	firefly := testDistributorChannel(59, constant.ChannelTypeFirefly)
	deps := testDistributorDependencies()
	deps.getChannelByID = func(int, bool) (*model.Channel, error) {
		return firefly, nil
	}

	status, selectedType, called, body := runDistributorRequest(t, http.MethodPost, "/v1/images/generations", deps, firefly.Id)
	if status != http.StatusNoContent || !called {
		t.Fatalf("status=%d called=%v, want Firefly image request allowed; body=%s", status, called, body)
	}
	if selectedType != constant.ChannelTypeFirefly {
		t.Fatalf("selected channel type = %d, want Firefly", selectedType)
	}
}

func TestSetupContextRejectsFireflyForChatRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	err := SetupContextForSelectedChannel(c, testDistributorChannel(59, constant.ChannelTypeFirefly), "gemini-3-pro-image")
	if err == nil {
		t.Fatal("SetupContextForSelectedChannel accepted Firefly for chat retry")
	}
	if channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType); channelType != 0 {
		t.Fatalf("channel type context = %d, want unset", channelType)
	}
}

func testDistributorDependencies() distributorDependencies {
	return distributorDependencies{
		getChannelByID: func(int, bool) (*model.Channel, error) {
			return nil, errors.New("unexpected fixed-channel lookup")
		},
		getCachedChannel: func(int) (*model.Channel, error) {
			return nil, errors.New("unexpected cached-channel lookup")
		},
		getPreferredChannelByAffinity: func(*gin.Context, string, string) (int, bool) {
			return 0, false
		},
		isChannelEnabledForGroupModel: func(string, string, int) bool {
			return true
		},
		getRandomSatisfiedChannel: func(*service.RetryParam) (*model.Channel, string, error) {
			return nil, "default", nil
		},
		getEnabledChannelsForGroupModel: func(string, string) ([]*model.Channel, error) {
			return nil, nil
		},
	}
}

func testDistributorChannel(id, channelType int) *model.Channel {
	baseURL := "http://upstream.example"
	weight := uint(1)
	priority := int64(0)
	return &model.Channel{
		Id:       id,
		Type:     channelType,
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		Name:     "test-channel",
		Weight:   &weight,
		Priority: &priority,
		BaseURL:  &baseURL,
		Models:   "gemini-3-pro-image",
		Group:    "default",
	}
}

func runDistributorRequest(t *testing.T, method, path string, deps distributorDependencies, fixedChannelID int) (status, selectedType int, called bool, body string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		if fixedChannelID > 0 {
			common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(fixedChannelID))
		}
		c.Next()
	})
	router.Use(distribute(deps))
	router.Handle(method, path, func(c *gin.Context) {
		called = true
		selectedType = common.GetContextKeyInt(c, constant.ContextKeyChannelType)
		c.Status(http.StatusNoContent)
	})

	requestBody := `{"model":"gemini-3-pro-image","prompt":"draw a cat"}`
	request := httptest.NewRequest(method, path, strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder.Code, selectedType, called, recorder.Body.String()
}
