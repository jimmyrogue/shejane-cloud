package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	SheJaneTelemetryTokenTTL   = 30 * 24 * time.Hour
	sheJaneTelemetryTokenBytes = 32
)

var (
	ErrSheJaneTelemetryUnauthorized = errors.New("invalid SheJane diagnostics credential")
	ErrSheJaneTelemetryInvalidEvent = errors.New("invalid SheJane diagnostics event")
	ErrSheJaneTelemetryUnavailable  = errors.New("SheJane diagnostics relay unavailable")

	SheJaneLangSmithAPIKey      = strings.TrimSpace(os.Getenv("SHEJANE_LANGSMITH_API_KEY"))
	SheJaneLangSmithEndpoint    = strings.TrimSpace(os.Getenv("SHEJANE_LANGSMITH_ENDPOINT"))
	SheJaneLangSmithProject     = strings.TrimSpace(os.Getenv("SHEJANE_LANGSMITH_PROJECT"))
	SheJaneLangSmithWorkspaceID = strings.TrimSpace(os.Getenv("SHEJANE_LANGSMITH_WORKSPACE_ID"))

	sheJaneTelemetryHTTPClient = &http.Client{
		Timeout: 2 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	sheJaneTelemetryAttemptPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,96}:[0-9]{1,10}$`)
	sheJaneTelemetryReleasePattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
)

var sheJaneTelemetryToolNames = map[string]struct{}{
	"clipboard.read": {}, "clipboard.write": {}, "edit_file": {}, "environment.observe": {},
	"execute": {}, "glob": {}, "grep": {}, "image.edit": {}, "image.generate": {}, "ls": {},
	"memory.search": {}, "memory.write": {}, "office.add_image_to_slide": {}, "office.add_row": {},
	"office.add_slide": {}, "office.apply_style": {}, "office.create_pptx": {}, "office.delete_paragraph": {},
	"office.delete_slide": {}, "office.find_replace": {}, "office.insert_paragraph": {}, "office.merge_cells": {},
	"office.outline": {}, "office.read": {}, "office.read_range": {}, "office.read_slides": {},
	"office.reorder_slides": {}, "office.set_cell_format": {}, "office.set_cells": {}, "office.set_formula": {},
	"office.set_slide_bullets": {}, "office.set_slide_notes": {}, "office.set_slide_title": {},
	"office.update_paragraph": {}, "office.update_slide": {}, "open.file": {}, "open.url": {}, "pdf.inspect": {},
	"read_file": {}, "task": {}, "task.progress": {}, "task.verify": {}, "time.now": {}, "user.ask": {},
	"web.fetch": {}, "web.search": {}, "write_file": {}, "write_todos": {},
}

type SheJaneTelemetryCredential struct {
	TokenType      string `json:"token_type"`
	TelemetryToken string `json:"telemetry_token"`
	ExpiresAt      int64  `json:"expires_at"`
}

type SheJaneTelemetryEvent struct {
	SchemaVersion   int      `json:"schema_version"`
	EventID         string   `json:"event_id"`
	RunID           string   `json:"run_id"`
	AttemptID       string   `json:"attempt_id"`
	ReleaseVersion  string   `json:"release_version"`
	Platform        string   `json:"platform"`
	Status          string   `json:"status"`
	StartedAt       string   `json:"started_at"`
	EndedAt         string   `json:"ended_at"`
	DurationMS      int64    `json:"duration_ms"`
	ModelCategory   string   `json:"model_category"`
	ToolNames       []string `json:"tool_names"`
	InputTokens     int64    `json:"input_tokens"`
	OutputTokens    int64    `json:"output_tokens"`
	FailureCategory string   `json:"failure_category,omitempty"`
}

func IssueSheJaneTelemetryCredential(rawInferenceToken string) (*SheJaneTelemetryCredential, error) {
	if !strings.HasPrefix(rawInferenceToken, "sk-") || len(rawInferenceToken) == len("sk-") {
		return nil, ErrSheJaneTelemetryUnauthorized
	}
	inferenceKey := strings.TrimPrefix(rawInferenceToken, "sk-")
	if _, _, err := activeSheJaneInferenceCredential(model.DB, inferenceKey, time.Now().Unix()); err != nil {
		return nil, err
	}
	if _, err := sheJaneTelemetryEndpoint(); err != nil {
		return nil, err
	}
	random := make([]byte, sheJaneTelemetryTokenBytes)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	raw := "st-" + base64.RawURLEncoding.EncodeToString(random)
	now := time.Now().Unix()
	expiresAt := time.Now().Add(SheJaneTelemetryTokenTTL).Unix()
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		inference, device, err := activeSheJaneInferenceCredential(tx, inferenceKey, now)
		if err != nil {
			return err
		}
		return model.CreateSheJaneTelemetryTokenWithTx(tx, &model.SheJaneTelemetryToken{
			UserId: inference.UserId, DeviceId: device.Id, TokenHash: sheJaneTelemetryTokenHash(raw),
			CreatedAt: now, ExpiresAt: expiresAt,
		})
	})
	if err != nil {
		return nil, err
	}
	return &SheJaneTelemetryCredential{TokenType: "Bearer", TelemetryToken: raw, ExpiresAt: expiresAt}, nil
}

func activeSheJaneInferenceCredential(tx *gorm.DB, key string, now int64) (*model.Token, *model.SheJaneDevice, error) {
	var inference model.Token
	if err := tx.Where(&model.Token{Key: key}).First(&inference).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrSheJaneTelemetryUnauthorized
		}
		return nil, nil, err
	}
	if inference.Status != common.TokenStatusEnabled || (inference.ExpiredTime != -1 && inference.ExpiredTime <= now) {
		return nil, nil, ErrSheJaneTelemetryUnauthorized
	}
	var user model.User
	if err := tx.Select("status").Where("id = ?", inference.UserId).First(&user).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
		return nil, nil, ErrSheJaneTelemetryUnauthorized
	}
	if user.Status != common.UserStatusEnabled {
		return nil, nil, ErrSheJaneTelemetryUnauthorized
	}
	var device model.SheJaneDevice
	if err := tx.Where("user_id = ? AND token_id = ? AND revoked_at = 0", inference.UserId, inference.Id).First(&device).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
		return nil, nil, ErrSheJaneTelemetryUnauthorized
	}
	return &inference, &device, nil
}

func ValidateSheJaneTelemetryCredential(raw string) (*model.SheJaneTelemetryToken, error) {
	encoded := strings.TrimPrefix(raw, "st-")
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || !strings.HasPrefix(raw, "st-") || len(decoded) != sheJaneTelemetryTokenBytes || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		return nil, ErrSheJaneTelemetryUnauthorized
	}
	token, err := model.GetActiveSheJaneTelemetryToken(sheJaneTelemetryTokenHash(raw), time.Now().Unix())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSheJaneTelemetryUnauthorized
		}
		return nil, err
	}
	return token, nil
}

func ValidateSheJaneTelemetryEvent(event SheJaneTelemetryEvent) error {
	if event.SchemaVersion != 1 || event.EventID != event.RunID {
		return ErrSheJaneTelemetryInvalidEvent
	}
	if !canonicalSheJaneTelemetryUUID(event.EventID) {
		return ErrSheJaneTelemetryInvalidEvent
	}
	if !validSheJaneTelemetryAttempt(event.AttemptID) || len(event.ReleaseVersion) > 32 || !sheJaneTelemetryReleasePattern.MatchString(event.ReleaseVersion) {
		return ErrSheJaneTelemetryInvalidEvent
	}
	switch event.Platform {
	case "macos", "windows", "linux":
	default:
		return ErrSheJaneTelemetryInvalidEvent
	}
	switch event.Status {
	case "completed", "canceled":
		if event.FailureCategory != "" {
			return ErrSheJaneTelemetryInvalidEvent
		}
	case "failed", "cleanup_required":
		if !validSheJaneTelemetryFailureCategory(event.FailureCategory) {
			return ErrSheJaneTelemetryInvalidEvent
		}
	default:
		return ErrSheJaneTelemetryInvalidEvent
	}
	switch event.ModelCategory {
	case "openai_chat", "openai_responses", "anthropic_messages", "google_genai", "unknown":
	default:
		return ErrSheJaneTelemetryInvalidEvent
	}
	startedAt, startErr := time.Parse(time.RFC3339, event.StartedAt)
	endedAt, endErr := time.Parse(time.RFC3339, event.EndedAt)
	if startErr != nil || endErr != nil || endedAt.Before(startedAt) || event.DurationMS < 0 || event.DurationMS > int64((7*24*time.Hour)/time.Millisecond) {
		return ErrSheJaneTelemetryInvalidEvent
	}
	if event.InputTokens < 0 || event.InputTokens > 1_000_000_000 || event.OutputTokens < 0 || event.OutputTokens > 1_000_000_000 || len(event.ToolNames) > 100 {
		return ErrSheJaneTelemetryInvalidEvent
	}
	for _, name := range event.ToolNames {
		if _, ok := sheJaneTelemetryToolNames[name]; !ok {
			return ErrSheJaneTelemetryInvalidEvent
		}
	}
	return nil
}

func ForwardSheJaneTelemetry(ctx context.Context, event SheJaneTelemetryEvent) error {
	if err := ValidateSheJaneTelemetryEvent(event); err != nil {
		return err
	}
	endpoint, err := sheJaneTelemetryEndpoint()
	if err != nil {
		return err
	}
	payload := struct {
		ID        string         `json:"id"`
		Name      string         `json:"name"`
		RunType   string         `json:"run_type"`
		Inputs    map[string]any `json:"inputs"`
		Outputs   map[string]any `json:"outputs"`
		StartTime string         `json:"start_time"`
		EndTime   string         `json:"end_time"`
		Error     string         `json:"error,omitempty"`
		Extra     struct {
			Metadata SheJaneTelemetryEvent `json:"metadata"`
		} `json:"extra"`
		SessionName string `json:"session_name"`
	}{
		ID: event.RunID, Name: "SheJane Agent Run", RunType: "chain",
		Inputs: map[string]any{}, Outputs: map[string]any{},
		StartTime: event.StartedAt, EndTime: event.EndedAt,
		SessionName: SheJaneLangSmithProject,
	}
	payload.Extra.Metadata = event
	if event.Status == "failed" || event.Status == "cleanup_required" {
		payload.Error = event.FailureCategory
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/runs", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSheJaneTelemetryUnavailable, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", SheJaneLangSmithAPIKey)
	if SheJaneLangSmithWorkspaceID != "" {
		request.Header.Set("x-tenant-id", SheJaneLangSmithWorkspaceID)
	}
	response, err := sheJaneTelemetryHTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSheJaneTelemetryUnavailable, err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusConflict {
		return ErrSheJaneTelemetryUnavailable
	}
	return nil
}

func sheJaneTelemetryTokenHash(raw string) string {
	return common.GenerateHMACWithKey([]byte("shejane-telemetry-v1:"+common.SessionSecret), raw)
}

func sheJaneTelemetryEndpoint() (string, error) {
	if SheJaneLangSmithAPIKey == "" || SheJaneLangSmithProject == "" {
		return "", ErrSheJaneTelemetryUnavailable
	}
	parsed, err := url.ParseRequestURI(SheJaneLangSmithEndpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", ErrSheJaneTelemetryUnavailable
	}
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func validSheJaneTelemetryAttempt(value string) bool {
	if canonicalSheJaneTelemetryUUID(value) {
		return true
	}
	return sheJaneTelemetryAttemptPattern.MatchString(value)
}

func canonicalSheJaneTelemetryUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func validSheJaneTelemetryFailureCategory(value string) bool {
	switch value {
	case "auth", "cleanup", "configuration", "execution_cleanup_unconfirmed", "fatal", "model_output",
		"permission", "provider_unavailable", "quota", "transient", "unknown", "unknown_failure",
		"validation", "verification", "workspace":
		return true
	default:
		return false
	}
}
