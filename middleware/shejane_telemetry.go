package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func SheJaneInferenceTokenHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || !strings.HasPrefix(parts[1], "sk-") || len(parts[1]) == len("sk-") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("shejane_inference_token", parts[1])
		c.Next()
	}
}

func SheJaneTelemetryAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		token, err := service.ValidateSheJaneTelemetryCredential(parts[1])
		if err != nil {
			if errors.Is(err, service.ErrSheJaneTelemetryUnauthorized) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				return
			}
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "temporarily_unavailable"})
			return
		}
		c.Set("id", token.UserId)
		c.Set("shejane_telemetry_device_id", token.DeviceId)
		c.Next()
	}
}
