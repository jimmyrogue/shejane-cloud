package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSheJaneTelemetryHTTPContractUsesSeparateCredentialsAndStrictEvents(t *testing.T) {
	db, identity := setupSheJaneControllerTest(t)
	previousKey, previousEndpoint, previousProject := service.SheJaneLangSmithAPIKey, service.SheJaneLangSmithEndpoint, service.SheJaneLangSmithProject
	service.SheJaneLangSmithAPIKey = "service-key"
	service.SheJaneLangSmithEndpoint = "https://api.smith.langchain.com"
	service.SheJaneLangSmithProject = "shejane-beta"
	previousForward := forwardSheJaneTelemetry
	forwardSheJaneTelemetry = func(_ context.Context, event service.SheJaneTelemetryEvent) error {
		assert.Equal(t, "019fabaf-e535-74f2-aa69-3962a58f2d91", event.RunID)
		return nil
	}
	t.Cleanup(func() {
		service.SheJaneLangSmithAPIKey, service.SheJaneLangSmithEndpoint, service.SheJaneLangSmithProject = previousKey, previousEndpoint, previousProject
		forwardSheJaneTelemetry = previousForward
	})

	now := time.Now().Unix()
	managed := model.Token{
		UserId: identity.UserID, Key: "managedtokenkey", Name: "SheJane: Mac", Status: common.TokenStatusEnabled,
		CreatedTime: now, AccessedTime: now, ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, db.Create(&managed).Error)
	device := model.SheJaneDevice{
		UserId: identity.UserID, TokenId: managed.Id, ClientId: service.SheJaneClientID,
		Name: "Mac", Platform: "macos", AppVersion: "0.1.8", CreatedAt: now,
	}
	require.NoError(t, db.Create(&device).Error)
	personal := model.Token{
		UserId: identity.UserID, Key: "personaltokenkey", Name: "Personal", Status: common.TokenStatusEnabled,
		CreatedTime: now, AccessedTime: now, ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, db.Create(&personal).Error)

	router := gin.New()
	router.POST("/api/shejane/telemetry/token", middleware.DisableCache(), middleware.SheJaneInferenceTokenHeader(), IssueSheJaneTelemetryToken)
	router.POST("/api/shejane/telemetry/events", middleware.DisableCache(), middleware.SheJaneTelemetryAuth(), IngestSheJaneTelemetryEvent)

	issueRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/telemetry/token", nil)
	issueRequest.Header.Set("Authorization", "Bearer sk-"+managed.Key)
	issueResponse := httptest.NewRecorder()
	router.ServeHTTP(issueResponse, issueRequest)
	assert.Equal(t, http.StatusCreated, issueResponse.Code)
	assert.Contains(t, issueResponse.Header().Get("Cache-Control"), "no-store")
	var credential service.SheJaneTelemetryCredential
	require.NoError(t, common.Unmarshal(issueResponse.Body.Bytes(), &credential))
	assert.True(t, strings.HasPrefix(credential.TelemetryToken, "st-"))
	assert.NotContains(t, issueResponse.Body.String(), managed.Key)
	bareRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/telemetry/token", nil)
	bareRequest.Header.Set("Authorization", "sk-"+managed.Key)
	bareResponse := httptest.NewRecorder()
	router.ServeHTTP(bareResponse, bareRequest)
	assert.Equal(t, http.StatusUnauthorized, bareResponse.Code)
	assert.Contains(t, bareResponse.Header().Get("Cache-Control"), "no-store")

	personalRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/telemetry/token", nil)
	personalRequest.Header.Set("Authorization", "Bearer sk-"+personal.Key)
	personalResponse := httptest.NewRecorder()
	router.ServeHTTP(personalResponse, personalRequest)
	assert.Equal(t, http.StatusUnauthorized, personalResponse.Code)
	assert.JSONEq(t, `{"error":"unauthorized"}`, personalResponse.Body.String())
	assert.NotContains(t, personalResponse.Body.String(), personal.Key)

	unknownRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/telemetry/token", nil)
	unknownRequest.Header.Set("Authorization", "Bearer sk-unknown")
	unknownResponse := httptest.NewRecorder()
	router.ServeHTTP(unknownResponse, unknownRequest)
	assert.Equal(t, http.StatusUnauthorized, unknownResponse.Code)
	assert.JSONEq(t, `{"error":"unauthorized"}`, unknownResponse.Body.String())

	eventJSON := `{"schema_version":1,"event_id":"019fabaf-e535-74f2-aa69-3962a58f2d91","run_id":"019fabaf-e535-74f2-aa69-3962a58f2d91","attempt_id":"job-id:1","release_version":"0.1.8","platform":"macos","status":"failed","started_at":"2026-07-29T02:24:19Z","ended_at":"2026-07-29T02:24:20Z","duration_ms":1000,"model_category":"openai_chat","tool_names":["read_file"],"input_tokens":120,"output_tokens":30,"failure_category":"provider_unavailable"}`
	eventRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/telemetry/events", strings.NewReader(eventJSON))
	eventRequest.Header.Set("Authorization", "Bearer "+credential.TelemetryToken)
	eventRequest.Header.Set("Content-Type", "application/json")
	eventResponse := httptest.NewRecorder()
	router.ServeHTTP(eventResponse, eventRequest)
	assert.Equal(t, http.StatusAccepted, eventResponse.Code)
	assert.Empty(t, eventResponse.Body.String())

	wrongCredentialRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/telemetry/events", strings.NewReader(eventJSON))
	wrongCredentialRequest.Header.Set("Authorization", "Bearer sk-"+managed.Key)
	wrongCredentialRequest.Header.Set("Content-Type", "application/json")
	wrongCredentialResponse := httptest.NewRecorder()
	router.ServeHTTP(wrongCredentialResponse, wrongCredentialRequest)
	assert.Equal(t, http.StatusUnauthorized, wrongCredentialResponse.Code)

	extraFieldRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/telemetry/events", strings.NewReader(strings.TrimSuffix(eventJSON, "}")+`,"prompt":"secret"}`))
	extraFieldRequest.Header.Set("Authorization", "Bearer "+credential.TelemetryToken)
	extraFieldRequest.Header.Set("Content-Type", "application/json")
	extraFieldResponse := httptest.NewRecorder()
	router.ServeHTTP(extraFieldResponse, extraFieldRequest)
	assert.Equal(t, http.StatusBadRequest, extraFieldResponse.Code)
	assert.NotContains(t, extraFieldResponse.Body.String(), "secret")

	invalidEvents := []string{
		strings.Replace(eventJSON, `"schema_version":1,`, `"schema_version":1,"event_id":"urn:uuid:019fabaf-e535-74f2-aa69-3962a58f2d91",`, 1),
		strings.Replace(eventJSON, `"duration_ms":1000`, `"duration_ms":null`, 1),
		strings.Replace(eventJSON, `"tool_names":["read_file"]`, `"tool_names":null`, 1),
		strings.Replace(eventJSON, `"input_tokens":120`, `"input_tokens":null`, 1),
	}
	for _, invalidEvent := range invalidEvents {
		request := httptest.NewRequest(http.MethodPost, "/api/shejane/telemetry/events", strings.NewReader(invalidEvent))
		request.Header.Set("Authorization", "Bearer "+credential.TelemetryToken)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	}

	oversizedRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/telemetry/events", bytes.NewReader(bytes.Repeat([]byte("x"), int(SheJaneTelemetryMaxBodyBytes+1))))
	oversizedRequest.Header.Set("Authorization", "Bearer "+credential.TelemetryToken)
	oversizedRequest.Header.Set("Content-Type", "application/json")
	oversizedResponse := httptest.NewRecorder()
	router.ServeHTTP(oversizedResponse, oversizedRequest)
	assert.Equal(t, http.StatusRequestEntityTooLarge, oversizedResponse.Code)

	_, err := service.RevokeSheJaneDevice(identity, device.Id)
	require.NoError(t, err)
	revokedRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/telemetry/events", strings.NewReader(eventJSON))
	revokedRequest.Header.Set("Authorization", "Bearer "+credential.TelemetryToken)
	revokedRequest.Header.Set("Content-Type", "application/json")
	revokedResponse := httptest.NewRecorder()
	router.ServeHTTP(revokedResponse, revokedRequest)
	assert.Equal(t, http.StatusUnauthorized, revokedResponse.Code)
}
