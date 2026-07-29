package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetiredFrontendAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	_, hasAsyncCleanup := routes[http.MethodPost+" /api/system-task/log-cleanup"]
	_, hasDirectDelete := routes[http.MethodDelete+" /api/log/"]
	_, hasConsoleMigration := routes[http.MethodPost+" /api/option/migrate_console_setting"]
	assert.True(t, hasAsyncCleanup)
	assert.False(t, hasDirectDelete)
	assert.False(t, hasConsoleMigration)
}

func TestSheJaneAuthorizationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	routes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		"POST /api/shejane/authorize/start",
		"GET /api/shejane/authorize/:flow_token",
		"POST /api/shejane/authorize/:flow_token",
		"POST /api/shejane/token",
		"GET /api/shejane/devices",
		"DELETE /api/shejane/devices/:id",
	} {
		assert.Contains(t, routes, route)
	}
}

func TestSheJaneAuthorizationPageDisablesCachingAndReferrers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sheJaneAuthorizationPageSecurityHeaders())
	engine.GET("/shejane/authorize", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/shejane/authorize?flow_token=opaque", nil)
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
	assert.Equal(t, "no-cache", recorder.Header().Get("Pragma"))
	assert.Equal(t, "no-referrer", recorder.Header().Get("Referrer-Policy"))
}

func TestSheJanePublicRoutesEnforceBodyAndCriticalRateLimits(t *testing.T) {
	previousBodyLimit := constant.AnonymousRequestBodyLimitKB
	previousRateEnabled := common.CriticalRateLimitEnable
	previousRateLimit := common.CriticalRateLimitNum
	previousRateDuration := common.CriticalRateLimitDuration
	previousRedis := common.RedisEnabled
	t.Cleanup(func() {
		constant.AnonymousRequestBodyLimitKB = previousBodyLimit
		common.CriticalRateLimitEnable = previousRateEnabled
		common.CriticalRateLimitNum = previousRateLimit
		common.CriticalRateLimitDuration = previousRateDuration
		common.RedisEnabled = previousRedis
	})

	constant.AnonymousRequestBodyLimitKB = 1
	common.CriticalRateLimitEnable = false
	bodyLimitRouter := gin.New()
	SetApiRouter(bodyLimitRouter)
	bodyRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/authorize/start", strings.NewReader(strings.Repeat("x", 1025)))
	bodyRequest.Header.Set("Content-Type", "application/json")
	bodyResponse := httptest.NewRecorder()
	bodyLimitRouter.ServeHTTP(bodyResponse, bodyRequest)
	assert.Equal(t, http.StatusRequestEntityTooLarge, bodyResponse.Code)

	constant.AnonymousRequestBodyLimitKB = 512
	common.CriticalRateLimitEnable = true
	common.RedisEnabled = false
	common.CriticalRateLimitNum = 1
	common.CriticalRateLimitDuration = 60
	rateRouter := gin.New()
	SetApiRouter(rateRouter)
	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/shejane/token", strings.NewReader("grant_type=invalid"))
		request.RemoteAddr = "198.51.100.71:4567"
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		rateRouter.ServeHTTP(response, request)
		if attempt == 0 {
			assert.Equal(t, http.StatusBadRequest, response.Code)
			continue
		}
		assert.Equal(t, http.StatusTooManyRequests, response.Code)
		require.NotEmpty(t, response.Header().Get("Retry-After"))
		assert.Empty(t, response.Body.String())
	}
}
