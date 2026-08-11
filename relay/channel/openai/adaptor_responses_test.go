package openai

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesRelayPreservesHostedWebSearch(t *testing.T) {
	request := dto.OpenAIResponsesRequest{
		Model:   "gpt-5.6-luna",
		Tools:   []byte(`[{"type":"web_search"}]`),
		Include: []byte(`["web_search_call.action.sources"]`),
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(
		nil,
		&relaycommon.RelayInfo{},
		request,
	)

	require.NoError(t, err)
	responsesRequest, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	assert.JSONEq(t, `[{"type":"web_search"}]`, string(responsesRequest.Tools))
	assert.JSONEq(t, `["web_search_call.action.sources"]`, string(responsesRequest.Include))
}
