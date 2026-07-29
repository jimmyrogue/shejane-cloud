package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSheJaneControllerTest(t *testing.T) (*gorm.DB, service.AuthIdentity) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuthFlow{}, &model.Token{}, &model.SheJaneDevice{}, &model.SheJaneTelemetryToken{}, &model.Log{}))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	user := model.User{
		Username: "controller-shejane", Password: "unused", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, Group: "default", AuthVersion: 1, AffCode: "controller-shejane-aff",
	}
	require.NoError(t, db.Create(&user).Error)
	now := time.Now().Unix()
	session := model.UserSession{
		SID: "controller-shejane-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: strings.Repeat("a", 64),
		LoginMethod: "password", CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, db.Create(&session).Error)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db, service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1}
}

func setSheJaneControllerIdentity(c *gin.Context, identity service.AuthIdentity) {
	c.Set("id", identity.UserID)
	c.Set("session_id", identity.SessionID)
	c.Set("auth_version", identity.UserAuthVersion)
	c.Set("session_version", identity.SessionVersion)
}

func sheJaneStartJSON(t *testing.T) []byte {
	t.Helper()
	payload, err := common.Marshal(service.SheJaneAuthorizationStartRequest{
		ResponseType: "code", ClientID: service.SheJaneClientID,
		RedirectURI:   "http://127.0.0.1:49152/shejane/auth/callback",
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", CodeChallengeMethod: "S256",
		State:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Device: service.SheJaneDeviceMetadata{Name: "Jimmy's Mac", Platform: "macos", AppVersion: "0.1.8"},
	})
	require.NoError(t, err)
	return payload
}

func TestSheJaneAuthorizationHTTPContractReturnsKeyOnlyFromSuccessfulExchange(t *testing.T) {
	db, identity := setupSheJaneControllerTest(t)

	startRecorder := httptest.NewRecorder()
	startContext, _ := gin.CreateTestContext(startRecorder)
	startContext.Request = httptest.NewRequest(http.MethodPost, "/api/shejane/authorize/start", bytes.NewReader(sheJaneStartJSON(t)))
	startContext.Request.Header.Set("Content-Type", "application/json")
	StartSheJaneAuthorization(startContext)
	assert.Equal(t, http.StatusCreated, startRecorder.Code)
	var startResponse struct {
		Data struct {
			FlowToken string `json:"flow_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(startRecorder.Body.Bytes(), &startResponse))
	require.NotEmpty(t, startResponse.Data.FlowToken)

	consentRecorder := httptest.NewRecorder()
	consentContext, _ := gin.CreateTestContext(consentRecorder)
	consentContext.Request = httptest.NewRequest(http.MethodGet, "/api/shejane/authorize/"+startResponse.Data.FlowToken, nil)
	consentContext.Params = gin.Params{{Key: "flow_token", Value: startResponse.Data.FlowToken}}
	setSheJaneControllerIdentity(consentContext, identity)
	GetSheJaneAuthorization(consentContext)
	assert.Equal(t, http.StatusOK, consentRecorder.Code)
	for _, secret := range []string{"redirect_uri", "state", "challenge", "verifier", "token_id", "session_id", "key"} {
		assert.NotContains(t, consentRecorder.Body.String(), secret)
	}

	decisionRecorder := httptest.NewRecorder()
	decisionContext, _ := gin.CreateTestContext(decisionRecorder)
	decisionContext.Request = httptest.NewRequest(http.MethodPost, "/api/shejane/authorize/"+startResponse.Data.FlowToken, strings.NewReader(`{"decision":"approve"}`))
	decisionContext.Request.Header.Set("Content-Type", "application/json")
	decisionContext.Params = gin.Params{{Key: "flow_token", Value: startResponse.Data.FlowToken}}
	setSheJaneControllerIdentity(decisionContext, identity)
	DecideSheJaneAuthorization(decisionContext)
	assert.Equal(t, http.StatusOK, decisionRecorder.Code)
	var decisionResponse struct {
		Data struct {
			RedirectTo string `json:"redirect_to"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(decisionRecorder.Body.Bytes(), &decisionResponse))
	callback, err := url.Parse(decisionResponse.Data.RedirectTo)
	require.NoError(t, err)
	code := callback.Query().Get("code")
	require.NotEmpty(t, code)

	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri":  {"http://127.0.0.1:49152/shejane/auth/callback"},
		"client_id":     {service.SheJaneClientID},
		"code_verifier": {"dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"},
	}
	router := gin.New()
	router.POST("/api/shejane/token", middleware.DisableCache(), ExchangeSheJaneAuthorization)
	exchangeRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/token", strings.NewReader(form.Encode()))
	exchangeRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	exchangeRecorder := httptest.NewRecorder()
	router.ServeHTTP(exchangeRecorder, exchangeRequest)
	assert.Equal(t, http.StatusOK, exchangeRecorder.Code)
	assert.Contains(t, exchangeRecorder.Header().Get("Cache-Control"), "no-store")
	assert.Equal(t, "no-cache", exchangeRecorder.Header().Get("Pragma"))
	var exchangeResponse struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, common.Unmarshal(exchangeRecorder.Body.Bytes(), &exchangeResponse))
	assert.True(t, strings.HasPrefix(exchangeResponse.AccessToken, "sk-"))
	assert.Equal(t, 1, strings.Count(exchangeRecorder.Body.String(), exchangeResponse.AccessToken))

	listRecorder := httptest.NewRecorder()
	listContext, _ := gin.CreateTestContext(listRecorder)
	listContext.Request = httptest.NewRequest(http.MethodGet, "/api/shejane/devices", nil)
	setSheJaneControllerIdentity(listContext, identity)
	GetSheJaneDevices(listContext)
	assert.Equal(t, http.StatusOK, listRecorder.Code)
	assert.NotContains(t, listRecorder.Body.String(), "token_id")
	assert.NotContains(t, listRecorder.Body.String(), exchangeResponse.AccessToken)
	var listResponse struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(listRecorder.Body.Bytes(), &listResponse))
	require.Len(t, listResponse.Data, 1)

	revokeRecorder := httptest.NewRecorder()
	revokeContext, _ := gin.CreateTestContext(revokeRecorder)
	revokeContext.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/shejane/devices/%d", listResponse.Data[0].ID), nil)
	revokeContext.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", listResponse.Data[0].ID)}}
	setSheJaneControllerIdentity(revokeContext, identity)
	DeleteSheJaneDevice(revokeContext)
	assert.Equal(t, http.StatusOK, revokeRecorder.Code)

	var auditLogs []model.Log
	require.NoError(t, db.Order("id").Find(&auditLogs).Error)
	require.Len(t, auditLogs, 2)
	for _, log := range auditLogs {
		for _, secret := range []string{exchangeResponse.AccessToken, code, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", identity.SessionID, "127.0.0.1:49152"} {
			assert.NotContains(t, log.Content+log.Other, secret)
		}
	}

	replayRecorder := httptest.NewRecorder()
	replayRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/token", strings.NewReader(form.Encode()))
	replayRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(replayRecorder, replayRequest)
	assert.Equal(t, http.StatusBadRequest, replayRecorder.Code)
	assert.JSONEq(t, `{"error":"invalid_grant"}`, replayRecorder.Body.String())
	assert.Contains(t, replayRecorder.Header().Get("Cache-Control"), "no-store")
	assert.NotContains(t, replayRecorder.Body.String(), exchangeResponse.AccessToken)
}

func TestSheJaneAuthorizationControllerRejectsPATForConsent(t *testing.T) {
	setupSheJaneControllerTest(t)
	start, err := service.StartSheJaneAuthorization(service.SheJaneAuthorizationStartRequest{
		ResponseType: "code", ClientID: service.SheJaneClientID,
		RedirectURI:   "http://127.0.0.1:49152/shejane/auth/callback",
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", CodeChallengeMethod: "S256",
		State:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Device: service.SheJaneDeviceMetadata{Name: "Mac", Platform: "macos", AppVersion: "0.1.8"},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/shejane/authorize/"+start.FlowToken, nil)
	context.Params = gin.Params{{Key: "flow_token", Value: start.FlowToken}}
	context.Set("id", 1)
	GetSheJaneAuthorization(context)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "AUTH_SESSION_REQUIRED")
}

func TestSheJaneAuthorizationErrorsDoNotTrustRequestRedirects(t *testing.T) {
	_, identity := setupSheJaneControllerTest(t)
	invalidPayload := sheJaneStartJSON(t)
	invalidPayload = bytes.Replace(invalidPayload, []byte(service.SheJaneClientID), []byte("untrusted-client"), 1)
	invalidRecorder := httptest.NewRecorder()
	invalidContext, _ := gin.CreateTestContext(invalidRecorder)
	invalidContext.Request = httptest.NewRequest(http.MethodPost, "/api/shejane/authorize/start", bytes.NewReader(invalidPayload))
	invalidContext.Request.Header.Set("Content-Type", "application/json")
	StartSheJaneAuthorization(invalidContext)
	assert.Equal(t, http.StatusBadRequest, invalidRecorder.Code)
	assert.Contains(t, invalidRecorder.Body.String(), "SHEJANE_INVALID_REQUEST")
	assert.NotContains(t, invalidRecorder.Body.String(), "redirect_to")

	start, err := service.StartSheJaneAuthorization(service.SheJaneAuthorizationStartRequest{
		ResponseType: "code", ClientID: service.SheJaneClientID,
		RedirectURI:   "http://127.0.0.1:49152/shejane/auth/callback",
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", CodeChallengeMethod: "S256",
		State:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Device: service.SheJaneDeviceMetadata{Name: "Mac", Platform: "macos", AppVersion: "0.1.8"},
	})
	require.NoError(t, err)
	denyRecorder := httptest.NewRecorder()
	denyContext, _ := gin.CreateTestContext(denyRecorder)
	denyContext.Request = httptest.NewRequest(http.MethodPost, "/api/shejane/authorize/"+start.FlowToken, strings.NewReader(`{"decision":"deny","redirect_uri":"https://attacker.example"}`))
	denyContext.Request.Header.Set("Content-Type", "application/json")
	denyContext.Params = gin.Params{{Key: "flow_token", Value: start.FlowToken}}
	setSheJaneControllerIdentity(denyContext, identity)
	DecideSheJaneAuthorization(denyContext)
	assert.Equal(t, http.StatusOK, denyRecorder.Code)
	assert.Contains(t, denyRecorder.Body.String(), "127.0.0.1:49152")
	assert.NotContains(t, denyRecorder.Body.String(), "attacker.example")

	router := gin.New()
	router.POST("/api/shejane/token", middleware.DisableCache(), ExchangeSheJaneAuthorization)
	wrongContentType := httptest.NewRequest(http.MethodPost, "/api/shejane/token", strings.NewReader(`{"code":"secret"}`))
	wrongContentType.Header.Set("Content-Type", "application/json")
	wrongContentTypeRecorder := httptest.NewRecorder()
	router.ServeHTTP(wrongContentTypeRecorder, wrongContentType)
	assert.Equal(t, http.StatusBadRequest, wrongContentTypeRecorder.Code)
	assert.JSONEq(t, `{"error":"invalid_request"}`, wrongContentTypeRecorder.Body.String())
	assert.Contains(t, wrongContentTypeRecorder.Header().Get("Cache-Control"), "no-store")
}

func TestSheJaneAuthorizationRouteRequiresLiveSessionAndTrustedOrigin(t *testing.T) {
	_, identity := setupSheJaneControllerTest(t)
	previousSecret := common.SessionSecret
	previousSecure := common.SessionCookieSecure
	previousTrusted := common.SessionCookieTrustedURLs
	common.SessionSecret = "shejane-route-contract-secret"
	common.SessionCookieSecure = true
	common.SessionCookieTrustedURLs = []string{"https://cloud.example"}
	t.Cleanup(func() {
		common.SessionSecret = previousSecret
		common.SessionCookieSecure = previousSecure
		common.SessionCookieTrustedURLs = previousTrusted
	})
	accessToken, _, err := service.IssueAccessToken(identity)
	require.NoError(t, err)

	start, err := service.StartSheJaneAuthorization(service.SheJaneAuthorizationStartRequest{
		ResponseType: "code", ClientID: service.SheJaneClientID,
		RedirectURI:   "http://127.0.0.1:49152/shejane/auth/callback",
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", CodeChallengeMethod: "S256",
		State:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Device: service.SheJaneDeviceMetadata{Name: "Mac", Platform: "macos", AppVersion: "0.1.8"},
	})
	require.NoError(t, err)

	router := gin.New()
	router.GET("/api/shejane/authorize/:flow_token", middleware.UserAuth(), middleware.DisableCache(), GetSheJaneAuthorization)
	router.POST("/api/shejane/authorize/:flow_token", middleware.UserAuth(), middleware.SessionCookieOriginGuard(), DecideSheJaneAuthorization)

	readRequest := httptest.NewRequest(http.MethodGet, "/api/shejane/authorize/"+start.FlowToken, nil)
	readRequest.Header.Set("Authorization", "Bearer "+accessToken)
	readResponse := httptest.NewRecorder()
	router.ServeHTTP(readResponse, readRequest)
	assert.Equal(t, http.StatusOK, readResponse.Code)
	assert.Contains(t, readResponse.Header().Get("Cache-Control"), "no-store")

	decisionRequest := httptest.NewRequest(http.MethodPost, "/api/shejane/authorize/"+start.FlowToken, strings.NewReader(`{"decision":"approve"}`))
	decisionRequest.Header.Set("Authorization", "Bearer "+accessToken)
	decisionRequest.Header.Set("Content-Type", "application/json")
	decisionRequest.Header.Set("Origin", "https://attacker.example")
	decisionResponse := httptest.NewRecorder()
	router.ServeHTTP(decisionResponse, decisionRequest)
	assert.Equal(t, http.StatusForbidden, decisionResponse.Code)
	assert.Contains(t, decisionResponse.Body.String(), "AUTH_ORIGIN_FORBIDDEN")
}
