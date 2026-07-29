package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	testSheJaneVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	testSheJaneChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	testSheJaneState     = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
)

func setupSheJaneAuthorizationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuthFlow{}, &model.Token{}, &model.SheJaneDevice{}, &model.SheJaneTelemetryToken{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func approveSheJaneAuthorization(t *testing.T, identity AuthIdentity) string {
	t.Helper()
	pending, err := StartSheJaneAuthorization(validSheJaneStartRequest())
	require.NoError(t, err)
	decision, err := DecideSheJaneAuthorization(pending.FlowToken, "approve", identity)
	require.NoError(t, err)
	callback, err := url.Parse(decision.RedirectTo)
	require.NoError(t, err)
	code := callback.Query().Get("code")
	require.NotEmpty(t, code)
	return code
}

func createSheJaneAuthorizationSession(t *testing.T, db *gorm.DB) AuthIdentity {
	t.Helper()
	var userCount int64
	require.NoError(t, db.Model(&model.User{}).Count(&userCount).Error)
	user := model.User{
		Username: fmt.Sprintf("shejane-user-%d", userCount+1), Password: "unused", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, Group: "default", AuthVersion: 1, AffCode: fmt.Sprintf("shejane-aff-%d", userCount+1),
	}
	require.NoError(t, db.Create(&user).Error)
	now := time.Now().Unix()
	session := model.UserSession{
		SID: fmt.Sprintf("shejane-session-%d", user.Id), UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: strings.Repeat("a", 64),
		LoginMethod: "password", CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, db.Create(&session).Error)
	return AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1}
}

func useSheJaneAuthorizationRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	previousEnabled, previousRDB := common.RedisEnabled, common.RDB
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		_ = client.Close()
		common.RedisEnabled = previousEnabled
		common.RDB = previousRDB
	})
	return server
}

func validSheJaneStartRequest() SheJaneAuthorizationStartRequest {
	return SheJaneAuthorizationStartRequest{
		ResponseType:        "code",
		ClientID:            "shejane-desktop",
		RedirectURI:         "http://127.0.0.1:49152/shejane/auth/callback",
		CodeChallenge:       testSheJaneChallenge,
		CodeChallengeMethod: "S256",
		State:               testSheJaneState,
		Device: SheJaneDeviceMetadata{
			Name:       "Jimmy's Mac",
			Platform:   "macos",
			AppVersion: "0.1.8",
		},
	}
}

func TestStartSheJaneAuthorizationValidatesFixedClientPKCEAndLoopbackRedirect(t *testing.T) {
	setupSheJaneAuthorizationTestDB(t)

	result, err := StartSheJaneAuthorization(validSheJaneStartRequest())
	require.NoError(t, err)
	assert.NotEmpty(t, result.FlowToken)
	assert.Greater(t, result.ExpiresAt, int64(0))

	tests := []struct {
		name   string
		mutate func(*SheJaneAuthorizationStartRequest)
	}{
		{name: "wrong response type", mutate: func(v *SheJaneAuthorizationStartRequest) { v.ResponseType = "token" }},
		{name: "wrong client", mutate: func(v *SheJaneAuthorizationStartRequest) { v.ClientID = "other" }},
		{name: "plain PKCE", mutate: func(v *SheJaneAuthorizationStartRequest) { v.CodeChallengeMethod = "plain" }},
		{name: "padded challenge", mutate: func(v *SheJaneAuthorizationStartRequest) { v.CodeChallenge += "=" }},
		{name: "short challenge", mutate: func(v *SheJaneAuthorizationStartRequest) { v.CodeChallenge = v.CodeChallenge[:42] }},
		{name: "localhost", mutate: func(v *SheJaneAuthorizationStartRequest) {
			v.RedirectURI = "http://localhost:49152/shejane/auth/callback"
		}},
		{name: "IPv6 loopback", mutate: func(v *SheJaneAuthorizationStartRequest) { v.RedirectURI = "http://[::1]:49152/shejane/auth/callback" }},
		{name: "remote host", mutate: func(v *SheJaneAuthorizationStartRequest) {
			v.RedirectURI = "http://example.com:49152/shejane/auth/callback"
		}},
		{name: "missing port", mutate: func(v *SheJaneAuthorizationStartRequest) { v.RedirectURI = "http://127.0.0.1/shejane/auth/callback" }},
		{name: "userinfo", mutate: func(v *SheJaneAuthorizationStartRequest) {
			v.RedirectURI = "http://user@127.0.0.1:49152/shejane/auth/callback"
		}},
		{name: "query", mutate: func(v *SheJaneAuthorizationStartRequest) { v.RedirectURI += "?next=1" }},
		{name: "fragment", mutate: func(v *SheJaneAuthorizationStartRequest) { v.RedirectURI += "#fragment" }},
		{name: "alternate path", mutate: func(v *SheJaneAuthorizationStartRequest) { v.RedirectURI = "http://127.0.0.1:49152/callback" }},
		{name: "https", mutate: func(v *SheJaneAuthorizationStartRequest) {
			v.RedirectURI = "https://127.0.0.1:49152/shejane/auth/callback"
		}},
		{name: "short state", mutate: func(v *SheJaneAuthorizationStartRequest) { v.State = "short" }},
		{name: "invalid state alphabet", mutate: func(v *SheJaneAuthorizationStartRequest) { v.State = strings.Repeat("+", 43) }},
		{name: "blank device name", mutate: func(v *SheJaneAuthorizationStartRequest) { v.Device.Name = "  " }},
		{name: "control in device name", mutate: func(v *SheJaneAuthorizationStartRequest) { v.Device.Name = "Mac\nBook" }},
		{name: "unsupported platform", mutate: func(v *SheJaneAuthorizationStartRequest) { v.Device.Platform = "ios" }},
		{name: "blank app version", mutate: func(v *SheJaneAuthorizationStartRequest) { v.Device.AppVersion = "" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validSheJaneStartRequest()
			test.mutate(&request)
			_, err := StartSheJaneAuthorization(request)
			assert.ErrorIs(t, err, ErrSheJaneInvalidRequest)
		})
	}
}

func TestSheJanePendingFlowApprovalAndDenialLifecycle(t *testing.T) {
	db := setupSheJaneAuthorizationTestDB(t)
	identity := createSheJaneAuthorizationSession(t, db)

	pending, err := StartSheJaneAuthorization(validSheJaneStartRequest())
	require.NoError(t, err)
	consent, err := ReadSheJaneAuthorization(pending.FlowToken, identity)
	require.NoError(t, err)
	assert.Equal(t, SheJaneClientID, consent.Client.ID)
	assert.Equal(t, SheJaneClientName, consent.Client.Name)
	assert.Equal(t, "Jimmy's Mac", consent.Device.Name)

	approved, err := DecideSheJaneAuthorization(pending.FlowToken, "approve", identity)
	require.NoError(t, err)
	callback, err := url.Parse(approved.RedirectTo)
	require.NoError(t, err)
	code := callback.Query().Get("code")
	assert.NotEmpty(t, code)
	assert.NotEqual(t, pending.FlowToken, code)
	assert.Equal(t, testSheJaneState, callback.Query().Get("state"))

	codeFlow, err := model.GetAuthFlow(code, model.AuthFlowMatch{
		Purpose: model.AuthFlowPurposeSheJaneAppAuthorization,
		Intent:  model.AuthFlowIntentSheJaneCode,
		UserId:  identity.UserID, SessionId: identity.SessionID,
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, time.Until(codeFlow.ExpiresAt), SheJaneCodeTTL)
	assert.Greater(t, time.Until(codeFlow.ExpiresAt), SheJaneCodeTTL-time.Second)

	_, err = DecideSheJaneAuthorization(pending.FlowToken, "approve", identity)
	assert.ErrorIs(t, err, ErrSheJaneFlowInvalid)

	deniedPending, err := StartSheJaneAuthorization(validSheJaneStartRequest())
	require.NoError(t, err)
	denied, err := DecideSheJaneAuthorization(deniedPending.FlowToken, "deny", identity)
	require.NoError(t, err)
	deniedCallback, err := url.Parse(denied.RedirectTo)
	require.NoError(t, err)
	assert.Equal(t, "access_denied", deniedCallback.Query().Get("error"))
	assert.Equal(t, testSheJaneState, deniedCallback.Query().Get("state"))
	assert.Empty(t, deniedCallback.Query().Get("code"))
}

func TestSheJaneConsentRequiresTheBoundLiveSession(t *testing.T) {
	db := setupSheJaneAuthorizationTestDB(t)
	identity := createSheJaneAuthorizationSession(t, db)
	pending, err := StartSheJaneAuthorization(validSheJaneStartRequest())
	require.NoError(t, err)

	wrongSession := identity
	wrongSession.SessionID = "other-session"
	_, err = ReadSheJaneAuthorization(pending.FlowToken, wrongSession)
	assert.ErrorIs(t, err, ErrSheJaneSessionInvalid)

	require.NoError(t, db.Model(&model.UserSession{}).Where("sid = ?", identity.SessionID).Update("status", model.UserSessionStatusRevoked).Error)
	_, err = DecideSheJaneAuthorization(pending.FlowToken, "approve", identity)
	assert.ErrorIs(t, err, ErrSheJaneSessionInvalid)

	flow, err := model.GetAuthFlow(pending.FlowToken, model.AuthFlowMatch{
		Purpose: model.AuthFlowPurposeSheJaneAppAuthorization,
		Intent:  model.AuthFlowIntentSheJanePending,
	})
	require.NoError(t, err)
	assert.Nil(t, flow.ConsumedAt)
}

func TestExchangeSheJaneAuthorizationIsSingleUseAndCreatesExactToken(t *testing.T) {
	db := setupSheJaneAuthorizationTestDB(t)
	identity := createSheJaneAuthorizationSession(t, db)
	code := approveSheJaneAuthorization(t, identity)

	request := SheJaneTokenExchangeRequest{
		GrantType: "authorization_code", Code: code,
		RedirectURI: validSheJaneStartRequest().RedirectURI,
		ClientID:    SheJaneClientID, CodeVerifier: testSheJaneVerifier,
	}
	result, err := ExchangeSheJaneAuthorization(request)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(result.AccessToken, "sk-"))
	assert.Equal(t, "Bearer", result.TokenType)

	var token model.Token
	require.NoError(t, db.First(&token).Error)
	assert.Equal(t, result.AccessToken, "sk-"+token.Key)
	assert.Equal(t, common.TokenStatusEnabled, token.Status)
	assert.True(t, token.UnlimitedQuota)
	assert.Zero(t, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Empty(t, token.Group)
	assert.False(t, token.ModelLimitsEnabled)
	assert.Empty(t, token.ModelLimits)
	assert.Equal(t, int64(-1), token.ExpiredTime)
	assert.NotNil(t, token.AllowIps)
	assert.Empty(t, *token.AllowIps)
	assert.False(t, token.CrossGroupRetry)

	var device model.SheJaneDevice
	require.NoError(t, db.First(&device).Error)
	assert.Equal(t, token.Id, device.TokenId)
	assert.Equal(t, identity.UserID, device.UserId)

	_, err = ExchangeSheJaneAuthorization(request)
	assert.ErrorIs(t, err, ErrSheJaneInvalidGrant)
	var tokenCount int64
	require.NoError(t, db.Model(&model.Token{}).Count(&tokenCount).Error)
	assert.Equal(t, int64(1), tokenCount)
}

func TestExchangeSheJaneAuthorizationRejectsMismatchWithoutConsumingCode(t *testing.T) {
	db := setupSheJaneAuthorizationTestDB(t)
	identity := createSheJaneAuthorizationSession(t, db)
	code := approveSheJaneAuthorization(t, identity)
	request := SheJaneTokenExchangeRequest{
		GrantType: "authorization_code", Code: code,
		RedirectURI: validSheJaneStartRequest().RedirectURI,
		ClientID:    SheJaneClientID, CodeVerifier: strings.Repeat("b", 43),
	}

	_, err := ExchangeSheJaneAuthorization(request)
	assert.ErrorIs(t, err, ErrSheJaneInvalidGrant)
	request.CodeVerifier = testSheJaneVerifier
	_, err = ExchangeSheJaneAuthorization(request)
	require.NoError(t, err)
}

func TestExchangeSheJaneAuthorizationEnforcesTokenLimitWithoutConsumingCode(t *testing.T) {
	db := setupSheJaneAuthorizationTestDB(t)
	identity := createSheJaneAuthorizationSession(t, db)
	code := approveSheJaneAuthorization(t, identity)
	setting := operation_setting.GetTokenSetting()
	previousLimit := setting.MaxUserTokens
	setting.MaxUserTokens = 1
	t.Cleanup(func() { setting.MaxUserTokens = previousLimit })
	require.NoError(t, db.Create(&model.Token{UserId: identity.UserID, Key: "existing", Name: "existing"}).Error)

	request := SheJaneTokenExchangeRequest{
		GrantType: "authorization_code", Code: code,
		RedirectURI: validSheJaneStartRequest().RedirectURI,
		ClientID:    SheJaneClientID, CodeVerifier: testSheJaneVerifier,
	}
	_, err := ExchangeSheJaneAuthorization(request)
	assert.ErrorIs(t, err, ErrSheJaneTokenLimit)

	require.NoError(t, db.Unscoped().Where("user_id = ?", identity.UserID).Delete(&model.Token{}).Error)
	_, err = ExchangeSheJaneAuthorization(request)
	require.NoError(t, err)
}

func TestExchangeSheJaneAuthorizationRollsBackCodeAndCredentialOnInsertFailure(t *testing.T) {
	tests := []struct {
		name       string
		schemaName string
	}{
		{name: "token insert", schemaName: "Token"},
		{name: "device insert", schemaName: "SheJaneDevice"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupSheJaneAuthorizationTestDB(t)
			identity := createSheJaneAuthorizationSession(t, db)
			code := approveSheJaneAuthorization(t, identity)
			request := SheJaneTokenExchangeRequest{
				GrantType: "authorization_code", Code: code, RedirectURI: validSheJaneStartRequest().RedirectURI,
				ClientID: SheJaneClientID, CodeVerifier: testSheJaneVerifier,
			}
			failInsert := true
			callbackName := "test:fail_shejane_" + strings.ReplaceAll(test.name, " ", "_")
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
				if failInsert && tx.Statement.Schema != nil && tx.Statement.Schema.Name == test.schemaName {
					tx.AddError(errors.New("injected insert failure"))
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

			_, err := ExchangeSheJaneAuthorization(request)
			require.Error(t, err)
			var tokenCount, deviceCount int64
			require.NoError(t, db.Model(&model.Token{}).Count(&tokenCount).Error)
			require.NoError(t, db.Model(&model.SheJaneDevice{}).Count(&deviceCount).Error)
			assert.Zero(t, tokenCount)
			assert.Zero(t, deviceCount)

			failInsert = false
			_, err = ExchangeSheJaneAuthorization(request)
			require.NoError(t, err)
		})
	}
}

func TestConcurrentSheJaneExchangeCreatesOneCredential(t *testing.T) {
	db := setupSheJaneAuthorizationTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	identity := createSheJaneAuthorizationSession(t, db)
	code := approveSheJaneAuthorization(t, identity)
	request := SheJaneTokenExchangeRequest{
		GrantType: "authorization_code", Code: code, RedirectURI: validSheJaneStartRequest().RedirectURI,
		ClientID: SheJaneClientID, CodeVerifier: testSheJaneVerifier,
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, exchangeErr := ExchangeSheJaneAuthorization(request)
			results <- exchangeErr
		}()
	}
	close(start)
	successes := 0
	for range 2 {
		if <-results == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes)
	var tokenCount, deviceCount int64
	require.NoError(t, db.Model(&model.Token{}).Count(&tokenCount).Error)
	require.NoError(t, db.Model(&model.SheJaneDevice{}).Count(&deviceCount).Error)
	assert.Equal(t, int64(1), tokenCount)
	assert.Equal(t, int64(1), deviceCount)
}

func TestExchangeSheJaneAuthorizationRejectsChangedUserAndSessionState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*gorm.DB, AuthIdentity) error
	}{
		{name: "disabled user", mutate: func(db *gorm.DB, identity AuthIdentity) error {
			return db.Model(&model.User{}).Where("id = ?", identity.UserID).Update("status", common.UserStatusDisabled).Error
		}},
		{name: "changed auth version", mutate: func(db *gorm.DB, identity AuthIdentity) error {
			return db.Model(&model.User{}).Where("id = ?", identity.UserID).Update("auth_version", 2).Error
		}},
		{name: "revoked session", mutate: func(db *gorm.DB, identity AuthIdentity) error {
			return db.Model(&model.UserSession{}).Where("sid = ?", identity.SessionID).Updates(map[string]interface{}{"status": model.UserSessionStatusRevoked, "revoked_at": time.Now().Unix()}).Error
		}},
		{name: "expired session", mutate: func(db *gorm.DB, identity AuthIdentity) error {
			return db.Model(&model.UserSession{}).Where("sid = ?", identity.SessionID).Update("expires_at", time.Now().Unix()-1).Error
		}},
		{name: "changed session version", mutate: func(db *gorm.DB, identity AuthIdentity) error {
			return db.Model(&model.UserSession{}).Where("sid = ?", identity.SessionID).Update("version", 2).Error
		}},
		{name: "changed session auth version", mutate: func(db *gorm.DB, identity AuthIdentity) error {
			return db.Model(&model.UserSession{}).Where("sid = ?", identity.SessionID).Update("user_auth_version", 2).Error
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupSheJaneAuthorizationTestDB(t)
			identity := createSheJaneAuthorizationSession(t, db)
			code := approveSheJaneAuthorization(t, identity)
			require.NoError(t, test.mutate(db, identity))
			_, err := ExchangeSheJaneAuthorization(SheJaneTokenExchangeRequest{
				GrantType: "authorization_code", Code: code, RedirectURI: validSheJaneStartRequest().RedirectURI,
				ClientID: SheJaneClientID, CodeVerifier: testSheJaneVerifier,
			})
			assert.ErrorIs(t, err, ErrSheJaneInvalidGrant)
			flow, flowErr := model.GetAuthFlow(code, model.AuthFlowMatch{
				Purpose: model.AuthFlowPurposeSheJaneAppAuthorization, Intent: model.AuthFlowIntentSheJaneCode,
			})
			require.NoError(t, flowErr)
			assert.Nil(t, flow.ConsumedAt)
		})
	}
}

func TestRevokeSheJaneDeviceIsOwnershipSafeIdempotentAndImmediate(t *testing.T) {
	db := setupSheJaneAuthorizationTestDB(t)
	owner := createSheJaneAuthorizationSession(t, db)
	otherUser := createSheJaneAuthorizationSession(t, db)
	code := approveSheJaneAuthorization(t, owner)
	exchange, err := ExchangeSheJaneAuthorization(SheJaneTokenExchangeRequest{
		GrantType: "authorization_code", Code: code, RedirectURI: validSheJaneStartRequest().RedirectURI,
		ClientID: SheJaneClientID, CodeVerifier: testSheJaneVerifier,
	})
	require.NoError(t, err)
	devices, err := ListSheJaneDevices(owner)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Zero(t, devices[0].TokenId)

	_, err = RevokeSheJaneDevice(otherUser, devices[0].Id)
	assert.ErrorIs(t, err, ErrSheJaneDeviceNotFound)

	useSheJaneAuthorizationRedis(t)
	rawKey := strings.TrimPrefix(exchange.AccessToken, "sk-")
	var token model.Token
	require.NoError(t, db.Where("key = ?", rawKey).First(&token).Error)
	require.NoError(t, common.RedisHSetObj("token:"+common.GenerateHMAC(rawKey), &token, time.Minute))

	revoked, err := RevokeSheJaneDevice(owner, devices[0].Id)
	require.NoError(t, err)
	assert.True(t, revoked.Revoked)
	_, err = model.ValidateUserToken(rawKey)
	assert.ErrorIs(t, err, model.ErrTokenInvalid)

	var stored model.Token
	require.NoError(t, db.Unscoped().First(&stored, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, stored.Status)
	assert.True(t, stored.DeletedAt.Valid)

	repeated, err := RevokeSheJaneDevice(owner, devices[0].Id)
	require.NoError(t, err)
	assert.True(t, repeated.Revoked)
}

func TestRevokeSheJaneDeviceFenceFailureLeavesDatabaseActive(t *testing.T) {
	db := setupSheJaneAuthorizationTestDB(t)
	owner := createSheJaneAuthorizationSession(t, db)
	code := approveSheJaneAuthorization(t, owner)
	_, err := ExchangeSheJaneAuthorization(SheJaneTokenExchangeRequest{
		GrantType: "authorization_code", Code: code, RedirectURI: validSheJaneStartRequest().RedirectURI,
		ClientID: SheJaneClientID, CodeVerifier: testSheJaneVerifier,
	})
	require.NoError(t, err)
	devices, err := ListSheJaneDevices(owner)
	require.NoError(t, err)
	require.Len(t, devices, 1)

	previousEnabled, previousRDB := common.RedisEnabled, common.RDB
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	require.NoError(t, client.Close())
	common.RedisEnabled, common.RDB = true, client
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousEnabled, previousRDB })

	_, err = RevokeSheJaneDevice(owner, devices[0].Id)
	require.Error(t, err)
	var device model.SheJaneDevice
	require.NoError(t, db.First(&device, devices[0].Id).Error)
	assert.Zero(t, device.RevokedAt)
	var activeTokens int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", owner.UserID).Count(&activeTokens).Error)
	assert.Equal(t, int64(1), activeTokens)
}
