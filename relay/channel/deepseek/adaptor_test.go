package deepseek

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepSeekResponsesUsesNativeEndpointAndReasoningContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	request := dto.OpenAIResponsesRequest{
		Model:              "deepseek-v4-flash-max",
		Tools:              []byte(`[{"type":"web_search","search_context_size":"high","user_location":{"country":"CN"}},{"type":"function","name":"lookup"}]`),
		Include:            []byte(`["web_search_call.action.sources"]`),
		Conversation:       []byte(`{"id":"conv-1"}`),
		PreviousResponseID: "resp-1",
		Store:              []byte(`false`),
		MaxToolCalls:       func(value uint) *uint { return &value }(3),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   constant.RelayModeResponses,
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://api.deepseek.com",
			UpstreamModelName: "deepseek-v4-flash-max",
		},
	}
	adaptor := &Adaptor{}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(ctx, info, request)
	require.NoError(t, err)
	responsesRequest, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	assert.Equal(t, "deepseek-v4-flash", responsesRequest.Model)
	require.NotNil(t, responsesRequest.Reasoning)
	assert.Equal(t, "max", responsesRequest.Reasoning.Effort)
	assert.JSONEq(t, `[{"type":"web_search"},{"type":"function","name":"lookup"}]`, string(responsesRequest.Tools))
	assert.Equal(t, json.RawMessage(nil), responsesRequest.Include)
	assert.Equal(t, json.RawMessage(nil), responsesRequest.Conversation)
	assert.Empty(t, responsesRequest.PreviousResponseID)
	assert.Equal(t, json.RawMessage(nil), responsesRequest.Store)
	assert.Nil(t, responsesRequest.MaxToolCalls)

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.deepseek.com/responses", url)
}

func TestDeepSeekV4BaseModelPreservesExplicitThinkingOptions(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:           "deepseek-v4-flash",
		THINKING:        []byte(`{"type":"enabled"}`),
		ReasoningEffort: "high",
	}

	err := applyDeepSeekV4OpenAIThinkingSuffix(&relaycommon.RelayInfo{}, request)

	require.NoError(t, err)
	assert.Equal(t, "deepseek-v4-flash", request.Model)
	assert.JSONEq(t, `{"type":"enabled"}`, string(request.THINKING))
	assert.Equal(t, "high", request.ReasoningEffort)
}

func TestDeepSeekV4AliasMapsToTheUpstreamThinkingContract(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{Model: "deepseek-v4-pro-none"}

	err := applyDeepSeekV4OpenAIThinkingSuffix(&relaycommon.RelayInfo{}, request)

	require.NoError(t, err)
	assert.Equal(t, "deepseek-v4-pro", request.Model)
	assert.JSONEq(t, `{"type":"disabled"}`, string(request.THINKING))
	assert.Empty(t, request.ReasoningEffort)
}
