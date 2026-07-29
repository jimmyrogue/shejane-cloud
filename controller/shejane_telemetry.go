package controller

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const SheJaneTelemetryMaxBodyBytes int64 = 32 * 1024

var forwardSheJaneTelemetry = service.ForwardSheJaneTelemetry

func IssueSheJaneTelemetryToken(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1))
	if err != nil || len(body) != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	credential, err := service.IssueSheJaneTelemetryCredential(c.GetString("shejane_inference_token"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSheJaneTelemetryUnauthorized):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		case errors.Is(err, service.ErrSheJaneTelemetryUnavailable):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporarily_unavailable"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		}
		return
	}
	c.JSON(http.StatusCreated, credential)
}

func IngestSheJaneTelemetryEvent(c *gin.Context) {
	mediaType, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_event"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, SheJaneTelemetryMaxBodyBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "event_too_large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_event"})
		return
	}
	var event service.SheJaneTelemetryEvent
	fields := map[string]bool{
		"schema_version": true, "event_id": true, "run_id": true, "attempt_id": true,
		"release_version": true, "platform": true, "status": true, "started_at": true,
		"ended_at": true, "duration_ms": true, "model_category": true, "tool_names": true,
		"input_tokens": true, "output_tokens": true, "failure_category": false,
	}
	if err := common.UnmarshalStrictObject(body, &event, fields); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_event"})
		return
	}
	if err := forwardSheJaneTelemetry(c.Request.Context(), event); err != nil {
		if errors.Is(err, service.ErrSheJaneTelemetryInvalidEvent) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_event"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporarily_unavailable"})
		return
	}
	c.Status(http.StatusAccepted)
}
