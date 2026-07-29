package service

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type telemetryRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip telemetryRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestSheJaneTelemetryCredentialIsSeparateAndFollowsDeviceRevocation(t *testing.T) {
	previousKey, previousEndpoint, previousProject := SheJaneLangSmithAPIKey, SheJaneLangSmithEndpoint, SheJaneLangSmithProject
	SheJaneLangSmithAPIKey = "service-key"
	SheJaneLangSmithEndpoint = "https://api.smith.langchain.com"
	SheJaneLangSmithProject = "shejane-beta"
	t.Cleanup(func() {
		SheJaneLangSmithAPIKey, SheJaneLangSmithEndpoint, SheJaneLangSmithProject = previousKey, previousEndpoint, previousProject
	})
	db := setupSheJaneAuthorizationTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SheJaneTelemetryToken{}))
	identity := createSheJaneAuthorizationSession(t, db)
	code := approveSheJaneAuthorization(t, identity)
	exchange, err := ExchangeSheJaneAuthorization(SheJaneTokenExchangeRequest{
		GrantType: "authorization_code", Code: code,
		RedirectURI: validSheJaneStartRequest().RedirectURI,
		ClientID:    SheJaneClientID, CodeVerifier: testSheJaneVerifier,
	})
	require.NoError(t, err)
	var inference model.Token
	require.NoError(t, db.Where("key = ?", strings.TrimPrefix(exchange.AccessToken, "sk-")).First(&inference).Error)

	credential, err := IssueSheJaneTelemetryCredential(exchange.AccessToken)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(credential.TelemetryToken, "st-"))
	assert.NotContains(t, credential.TelemetryToken, exchange.AccessToken)

	var stored model.SheJaneTelemetryToken
	require.NoError(t, db.First(&stored).Error)
	assert.NotContains(t, stored.TokenHash, credential.TelemetryToken)
	validated, err := ValidateSheJaneTelemetryCredential(credential.TelemetryToken)
	require.NoError(t, err)
	assert.Equal(t, identity.UserID, validated.UserId)

	personal := model.Token{UserId: identity.UserID, Key: "personal", Name: "personal", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(&personal).Error)
	_, err = IssueSheJaneTelemetryCredential("sk-" + personal.Key)
	assert.ErrorIs(t, err, ErrSheJaneTelemetryUnauthorized)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", inference.Id).Update("expired_time", time.Now().Add(-time.Minute).Unix()).Error)
	_, err = IssueSheJaneTelemetryCredential(exchange.AccessToken)
	assert.ErrorIs(t, err, ErrSheJaneTelemetryUnauthorized)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", inference.Id).Update("expired_time", -1).Error)

	devices, err := ListSheJaneDevices(identity)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	_, err = RevokeSheJaneDevice(identity, devices[0].Id)
	require.NoError(t, err)
	_, err = ValidateSheJaneTelemetryCredential(credential.TelemetryToken)
	assert.ErrorIs(t, err, ErrSheJaneTelemetryUnauthorized)
}

func TestSheJaneTelemetryMintAuthenticatesBeforeRelayAvailability(t *testing.T) {
	previousKey, previousEndpoint, previousProject := SheJaneLangSmithAPIKey, SheJaneLangSmithEndpoint, SheJaneLangSmithProject
	SheJaneLangSmithAPIKey = ""
	SheJaneLangSmithEndpoint = ""
	SheJaneLangSmithProject = ""
	t.Cleanup(func() {
		SheJaneLangSmithAPIKey, SheJaneLangSmithEndpoint, SheJaneLangSmithProject = previousKey, previousEndpoint, previousProject
	})
	setupSheJaneAuthorizationTestDB(t)

	_, err := IssueSheJaneTelemetryCredential("sk-unknown")
	assert.ErrorIs(t, err, ErrSheJaneTelemetryUnauthorized)
}

func TestForwardSheJaneTelemetryUsesOnlyAllowlistedLangSmithMetadata(t *testing.T) {
	previousClient := sheJaneTelemetryHTTPClient
	previousKey, previousEndpoint := SheJaneLangSmithAPIKey, SheJaneLangSmithEndpoint
	previousProject, previousWorkspace := SheJaneLangSmithProject, SheJaneLangSmithWorkspaceID
	t.Cleanup(func() {
		sheJaneTelemetryHTTPClient = previousClient
		SheJaneLangSmithAPIKey, SheJaneLangSmithEndpoint = previousKey, previousEndpoint
		SheJaneLangSmithProject, SheJaneLangSmithWorkspaceID = previousProject, previousWorkspace
	})
	SheJaneLangSmithAPIKey = "service-key"
	SheJaneLangSmithEndpoint = "https://api.smith.langchain.com"
	SheJaneLangSmithProject = "shejane-beta"
	SheJaneLangSmithWorkspaceID = "workspace-id"

	var captured []byte
	sheJaneTelemetryHTTPClient = &http.Client{Transport: telemetryRoundTripper(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://api.smith.langchain.com/runs", request.URL.String())
		assert.Equal(t, "service-key", request.Header.Get("x-api-key"))
		assert.Equal(t, "workspace-id", request.Header.Get("x-tenant-id"))
		var err error
		captured, err = io.ReadAll(request.Body)
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}

	event := SheJaneTelemetryEvent{
		SchemaVersion:   1,
		EventID:         "019fabaf-e535-74f2-aa69-3962a58f2d91",
		RunID:           "019fabaf-e535-74f2-aa69-3962a58f2d91",
		AttemptID:       "job-id:1",
		ReleaseVersion:  "0.1.8",
		Platform:        "macos",
		Status:          "failed",
		StartedAt:       "2026-07-29T02:24:19Z",
		EndedAt:         "2026-07-29T02:24:20Z",
		DurationMS:      1000,
		ModelCategory:   "openai_chat",
		ToolNames:       []string{"read_file"},
		InputTokens:     120,
		OutputTokens:    30,
		FailureCategory: "provider_unavailable",
	}
	require.NoError(t, ValidateSheJaneTelemetryEvent(event))
	require.NoError(t, ForwardSheJaneTelemetry(t.Context(), event))

	body := string(captured)
	for _, secret := range []string{"prompt", "tool result", "local file", "inference-token"} {
		assert.NotContains(t, body, secret)
	}
	assert.Contains(t, body, `"inputs":{}`)
	assert.Contains(t, body, `"outputs":{}`)
	assert.Contains(t, body, `"failure_category":"provider_unavailable"`)
	assert.Contains(t, body, `"session_name":"shejane-beta"`)

	invalid := event
	invalid.ToolNames = []string{"read_file", strings.Repeat("x", 65)}
	assert.ErrorIs(t, ValidateSheJaneTelemetryEvent(invalid), ErrSheJaneTelemetryInvalidEvent)
	invalid = event
	invalid.EndedAt = "2026-07-29T02:24:18Z"
	assert.ErrorIs(t, ValidateSheJaneTelemetryEvent(invalid), ErrSheJaneTelemetryInvalidEvent)
	invalid = event
	invalid.ToolNames = []string{"/Users/alice/private.txt"}
	assert.ErrorIs(t, ValidateSheJaneTelemetryEvent(invalid), ErrSheJaneTelemetryInvalidEvent)
	invalid = event
	invalid.FailureCategory = "https://example.test/private"
	assert.ErrorIs(t, ValidateSheJaneTelemetryEvent(invalid), ErrSheJaneTelemetryInvalidEvent)
	invalid = event
	invalid.ToolNames = []string{"private.txt"}
	assert.ErrorIs(t, ValidateSheJaneTelemetryEvent(invalid), ErrSheJaneTelemetryInvalidEvent)
	invalid = event
	invalid.FailureCategory = "urn:private"
	assert.ErrorIs(t, ValidateSheJaneTelemetryEvent(invalid), ErrSheJaneTelemetryInvalidEvent)
	invalid = event
	invalid.EventID = "urn:uuid:" + event.EventID
	invalid.RunID = invalid.EventID
	assert.ErrorIs(t, ValidateSheJaneTelemetryEvent(invalid), ErrSheJaneTelemetryInvalidEvent)
	invalid = event
	invalid.AttemptID = "urn:uuid:" + event.EventID
	assert.ErrorIs(t, ValidateSheJaneTelemetryEvent(invalid), ErrSheJaneTelemetryInvalidEvent)
}
