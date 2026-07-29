package controller

import (
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func StartSheJaneAuthorization(c *gin.Context) {
	var request service.SheJaneAuthorizationStartRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSheJaneError(c, http.StatusBadRequest, "SHEJANE_INVALID_REQUEST", "invalid authorization request")
		return
	}
	result, err := service.StartSheJaneAuthorization(request)
	if err != nil {
		if errors.Is(err, service.ErrSheJaneInvalidRequest) {
			writeSheJaneError(c, http.StatusBadRequest, "SHEJANE_INVALID_REQUEST", "invalid authorization request")
			return
		}
		writeSheJaneError(c, http.StatusInternalServerError, "SHEJANE_INTERNAL_ERROR", "authorization request failed")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "", "data": result})
}

func GetSheJaneAuthorization(c *gin.Context) {
	identity, ok := requireSheJaneSession(c)
	if !ok {
		return
	}
	result, err := service.ReadSheJaneAuthorization(c.Param("flow_token"), identity)
	if err != nil {
		writeSheJaneFlowError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}

func DecideSheJaneAuthorization(c *gin.Context) {
	identity, ok := requireSheJaneSession(c)
	if !ok {
		return
	}
	var request struct {
		Decision string `json:"decision"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSheJaneError(c, http.StatusBadRequest, "SHEJANE_INVALID_DECISION", "invalid authorization decision")
		return
	}
	result, err := service.DecideSheJaneAuthorization(c.Param("flow_token"), request.Decision, identity)
	if err != nil {
		writeSheJaneFlowError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}

func ExchangeSheJaneAuthorization(c *gin.Context) {
	setSheJaneExchangeNoStore(c)
	mediaType, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	var request service.SheJaneTokenExchangeRequest
	if err := c.ShouldBind(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	result, err := service.ExchangeSheJaneAuthorization(request)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSheJaneInvalidRequest):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		case errors.Is(err, service.ErrSheJaneInvalidGrant), errors.Is(err, service.ErrSheJaneTokenLimit):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		}
		return
	}
	recordUserSecurityAudit(c, result.Device.UserId, "user.shejane_device_authorized", map[string]interface{}{
		"device_id": result.Device.Id, "client_id": result.Device.ClientId,
		"platform": result.Device.Platform, "app_version": result.Device.AppVersion,
	})
	c.JSON(http.StatusOK, gin.H{"access_token": result.AccessToken, "token_type": result.TokenType})
}

func GetSheJaneDevices(c *gin.Context) {
	identity, ok := requireSheJaneSession(c)
	if !ok {
		return
	}
	devices, err := service.ListSheJaneDevices(identity)
	if err != nil {
		writeSheJaneFlowError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": devices})
}

func DeleteSheJaneDevice(c *gin.Context) {
	identity, ok := requireSheJaneSession(c)
	if !ok {
		return
	}
	deviceId, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || deviceId <= 0 {
		writeSheJaneError(c, http.StatusNotFound, "SHEJANE_DEVICE_NOT_FOUND", "device not found")
		return
	}
	result, err := service.RevokeSheJaneDevice(identity, deviceId)
	if err != nil {
		if errors.Is(err, service.ErrSheJaneDeviceNotFound) {
			writeSheJaneError(c, http.StatusNotFound, "SHEJANE_DEVICE_NOT_FOUND", "device not found")
			return
		}
		if errors.Is(err, service.ErrSheJaneSessionInvalid) {
			writeSheJaneError(c, http.StatusForbidden, "AUTH_SESSION_REQUIRED", "a live browser session is required")
			return
		}
		writeSheJaneError(c, http.StatusInternalServerError, "SHEJANE_INTERNAL_ERROR", "device revocation failed")
		return
	}
	recordUserSecurityAudit(c, identity.UserID, "user.shejane_device_revoked", map[string]interface{}{
		"device_id": result.Device.Id, "client_id": result.Device.ClientId,
		"platform": result.Device.Platform, "app_version": result.Device.AppVersion,
		"already_revoked": result.AlreadyRevoked,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}

func requireSheJaneSession(c *gin.Context) (service.AuthIdentity, bool) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		writeSheJaneError(c, http.StatusForbidden, "AUTH_SESSION_REQUIRED", "a live browser session is required")
		return service.AuthIdentity{}, false
	}
	return identity, true
}

func writeSheJaneFlowError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrSheJaneSessionInvalid):
		writeSheJaneError(c, http.StatusForbidden, "AUTH_SESSION_REQUIRED", "a live browser session is required")
	case errors.Is(err, service.ErrSheJaneFlowExpired):
		writeSheJaneError(c, http.StatusGone, "SHEJANE_FLOW_EXPIRED", "authorization flow expired")
	case errors.Is(err, service.ErrSheJaneFlowInvalid):
		writeSheJaneError(c, http.StatusNotFound, "SHEJANE_FLOW_INVALID", "authorization flow not found")
	case errors.Is(err, service.ErrSheJaneInvalidDecision):
		writeSheJaneError(c, http.StatusBadRequest, "SHEJANE_INVALID_DECISION", "invalid authorization decision")
	default:
		writeSheJaneError(c, http.StatusInternalServerError, "SHEJANE_INTERNAL_ERROR", "authorization request failed")
	}
}

func writeSheJaneError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"success": false, "code": code, "message": message})
}

func setSheJaneExchangeNoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}
