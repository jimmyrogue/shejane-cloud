package dto

import "github.com/QuantumNous/new-api/relaykit/types"

// 这里不好动就不动了，本来想独立出来的（
type OpenAIModels struct {
	Id                     string                       `json:"id"`
	Object                 string                       `json:"object"`
	Created                int                          `json:"created"`
	OwnedBy                string                       `json:"owned_by"`
	SupportedEndpointTypes []types.EndpointType         `json:"supported_endpoint_types"`
	Capabilities           []string                     `json:"capabilities,omitempty"`
	RecommendedFor         []string                     `json:"recommended_for,omitempty"`
	ProviderFamily         string                       `json:"provider_family,omitempty"`
	Reasoning              *ModelReasoningProfile       `json:"reasoning,omitempty"`
	HostedWebSearch        *ModelHostedWebSearchProfile `json:"hosted_web_search,omitempty"`
	MaxInputTokens         *int                         `json:"max_input_tokens,omitempty"`
	MaxOutputTokens        *int                         `json:"max_output_tokens,omitempty"`
}

type ModelHostedWebSearchProfile struct {
	Verification string `json:"verification"`
	FullSources  bool   `json:"full_sources"`
}

type ModelReasoningProfile struct {
	Supported             bool     `json:"supported"`
	Modes                 []string `json:"modes"`
	DefaultMode           string   `json:"default_mode"`
	StreamField           *string  `json:"stream_field"`
	ToolRoundtripRequired bool     `json:"tool_roundtrip_required"`
	DisplayPolicy         string   `json:"display_policy"`
}

type AnthropicModel struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

type GeminiModel struct {
	Name                       interface{}   `json:"name"`
	BaseModelId                interface{}   `json:"baseModelId"`
	Version                    interface{}   `json:"version"`
	DisplayName                interface{}   `json:"displayName"`
	Description                interface{}   `json:"description"`
	InputTokenLimit            interface{}   `json:"inputTokenLimit"`
	OutputTokenLimit           interface{}   `json:"outputTokenLimit"`
	SupportedGenerationMethods []interface{} `json:"supportedGenerationMethods"`
	Thinking                   interface{}   `json:"thinking"`
	Temperature                interface{}   `json:"temperature"`
	MaxTemperature             interface{}   `json:"maxTemperature"`
	TopP                       interface{}   `json:"topP"`
	TopK                       interface{}   `json:"topK"`
}
