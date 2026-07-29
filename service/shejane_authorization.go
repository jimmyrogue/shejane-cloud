package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

const (
	SheJaneClientID         = "shejane-desktop"
	SheJaneClientName       = "SheJane Desktop"
	SheJaneResponseTypeCode = "code"
	SheJanePKCEMethodS256   = "S256"
	SheJaneCallbackPath     = "/shejane/auth/callback"
	SheJanePendingTTL       = 10 * time.Minute
	SheJaneCodeTTL          = 2 * time.Minute
)

var (
	ErrSheJaneInvalidRequest  = errors.New("invalid SheJane authorization request")
	ErrSheJaneInvalidDecision = errors.New("invalid SheJane authorization decision")
	ErrSheJaneFlowInvalid     = errors.New("SheJane authorization flow is invalid")
	ErrSheJaneFlowExpired     = errors.New("SheJane authorization flow has expired")
	ErrSheJaneSessionInvalid  = errors.New("SheJane authorization requires a live browser session")
	ErrSheJaneInvalidGrant    = errors.New("invalid SheJane authorization grant")
	ErrSheJaneTokenLimit      = errors.New("SheJane authorization token limit reached")
	ErrSheJaneDeviceNotFound  = errors.New("SheJane device not found")
)

type SheJaneDeviceMetadata struct {
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
}

type SheJaneAuthorizationStartRequest struct {
	ResponseType        string                `json:"response_type"`
	ClientID            string                `json:"client_id"`
	RedirectURI         string                `json:"redirect_uri"`
	CodeChallenge       string                `json:"code_challenge"`
	CodeChallengeMethod string                `json:"code_challenge_method"`
	State               string                `json:"state"`
	Device              SheJaneDeviceMetadata `json:"device"`
}

type SheJaneAuthorizationStartResult struct {
	FlowToken string `json:"flow_token"`
	ExpiresAt int64  `json:"expires_at"`
}

type SheJaneAuthorizationClient struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SheJaneAuthorizationConsent struct {
	Client    SheJaneAuthorizationClient `json:"client"`
	Device    SheJaneDeviceMetadata      `json:"device"`
	ExpiresAt int64                      `json:"expires_at"`
}

type SheJaneAuthorizationDecisionResult struct {
	RedirectTo string `json:"redirect_to"`
}

type SheJaneTokenExchangeRequest struct {
	GrantType    string `form:"grant_type"`
	Code         string `form:"code"`
	RedirectURI  string `form:"redirect_uri"`
	ClientID     string `form:"client_id"`
	CodeVerifier string `form:"code_verifier"`
}

type SheJaneTokenExchangeResult struct {
	AccessToken string              `json:"access_token"`
	TokenType   string              `json:"token_type"`
	Device      model.SheJaneDevice `json:"-"`
}

type SheJaneDeviceRevocationResult struct {
	ID             int64               `json:"id"`
	Revoked        bool                `json:"revoked"`
	Device         model.SheJaneDevice `json:"-"`
	AlreadyRevoked bool                `json:"-"`
}

type sheJaneAuthorizationPayload struct {
	ClientID            string                `json:"client_id"`
	RedirectURI         string                `json:"redirect_uri"`
	CodeChallenge       string                `json:"code_challenge"`
	CodeChallengeMethod string                `json:"code_challenge_method"`
	State               string                `json:"state"`
	Device              SheJaneDeviceMetadata `json:"device"`
	UserAuthVersion     int64                 `json:"user_auth_version,omitempty"`
	SessionVersion      int64                 `json:"session_version,omitempty"`
}

func StartSheJaneAuthorization(request SheJaneAuthorizationStartRequest) (*SheJaneAuthorizationStartResult, error) {
	if err := validateSheJaneAuthorizationStart(&request); err != nil {
		return nil, err
	}
	payload, err := common.Marshal(sheJaneAuthorizationPayload{
		ClientID:            request.ClientID,
		RedirectURI:         request.RedirectURI,
		CodeChallenge:       request.CodeChallenge,
		CodeChallengeMethod: request.CodeChallengeMethod,
		State:               request.State,
		Device:              request.Device,
	})
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(SheJanePendingTTL)
	token, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   model.AuthFlowPurposeSheJaneAppAuthorization,
		Intent:    model.AuthFlowIntentSheJanePending,
		Payload:   string(payload),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, err
	}
	return &SheJaneAuthorizationStartResult{FlowToken: token, ExpiresAt: expiresAt.Unix()}, nil
}

func ReadSheJaneAuthorization(flowToken string, identity AuthIdentity) (*SheJaneAuthorizationConsent, error) {
	if _, _, err := ValidateLoginSession(identity); err != nil {
		return nil, ErrSheJaneSessionInvalid
	}
	flow, err := model.GetAuthFlow(flowToken, model.AuthFlowMatch{
		Purpose: model.AuthFlowPurposeSheJaneAppAuthorization,
		Intent:  model.AuthFlowIntentSheJanePending,
	})
	if err != nil {
		return nil, mapSheJaneFlowError(err)
	}
	var payload sheJaneAuthorizationPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		return nil, ErrSheJaneFlowInvalid
	}
	return &SheJaneAuthorizationConsent{
		Client:    SheJaneAuthorizationClient{ID: SheJaneClientID, Name: SheJaneClientName},
		Device:    payload.Device,
		ExpiresAt: flow.ExpiresAt.Unix(),
	}, nil
}

func DecideSheJaneAuthorization(flowToken, decision string, identity AuthIdentity) (*SheJaneAuthorizationDecisionResult, error) {
	if decision != "approve" && decision != "deny" {
		return nil, ErrSheJaneInvalidDecision
	}
	if _, _, err := ValidateLoginSession(identity); err != nil {
		return nil, ErrSheJaneSessionInvalid
	}
	var payload sheJaneAuthorizationPayload
	var code string
	_, err := model.ConsumeAuthFlowWithAction(flowToken, model.AuthFlowMatch{
		Purpose: model.AuthFlowPurposeSheJaneAppAuthorization,
		Intent:  model.AuthFlowIntentSheJanePending,
	}, func(tx *gorm.DB, flow *model.AuthFlow) error {
		if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
			return ErrSheJaneFlowInvalid
		}
		if decision == "deny" {
			return nil
		}
		payload.UserAuthVersion = identity.UserAuthVersion
		payload.SessionVersion = identity.SessionVersion
		encoded, err := common.Marshal(payload)
		if err != nil {
			return err
		}
		code, _, err = model.CreateAuthFlowWithTx(tx, model.AuthFlowCreate{
			Purpose:   model.AuthFlowPurposeSheJaneAppAuthorization,
			Intent:    model.AuthFlowIntentSheJaneCode,
			UserId:    identity.UserID,
			SessionId: identity.SessionID,
			Payload:   string(encoded),
			ExpiresAt: time.Now().Add(SheJaneCodeTTL),
		})
		return err
	})
	if err != nil {
		return nil, mapSheJaneFlowError(err)
	}
	callback, err := url.Parse(payload.RedirectURI)
	if err != nil {
		return nil, ErrSheJaneFlowInvalid
	}
	query := callback.Query()
	if decision == "approve" {
		query.Set("code", code)
	} else {
		query.Set("error", "access_denied")
	}
	query.Set("state", payload.State)
	callback.RawQuery = query.Encode()
	return &SheJaneAuthorizationDecisionResult{RedirectTo: callback.String()}, nil
}

func ExchangeSheJaneAuthorization(request SheJaneTokenExchangeRequest) (*SheJaneTokenExchangeResult, error) {
	if request.GrantType == "" || request.Code == "" || request.RedirectURI == "" || request.ClientID == "" || request.CodeVerifier == "" {
		return nil, ErrSheJaneInvalidRequest
	}
	if request.GrantType != "authorization_code" || !validSheJaneRedirectURI(request.RedirectURI) || !validSheJaneVerifier(request.CodeVerifier) {
		return nil, ErrSheJaneInvalidRequest
	}
	if request.ClientID != SheJaneClientID {
		return nil, ErrSheJaneInvalidGrant
	}
	var issuedToken model.Token
	var device model.SheJaneDevice
	_, err := model.ConsumeAuthFlowWithAction(request.Code, model.AuthFlowMatch{
		Purpose: model.AuthFlowPurposeSheJaneAppAuthorization,
		Intent:  model.AuthFlowIntentSheJaneCode,
	}, func(tx *gorm.DB, flow *model.AuthFlow) error {
		var payload sheJaneAuthorizationPayload
		if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
			return ErrSheJaneInvalidGrant
		}
		if payload.ClientID != request.ClientID || payload.RedirectURI != request.RedirectURI || payload.CodeChallengeMethod != SheJanePKCEMethodS256 {
			return ErrSheJaneInvalidGrant
		}
		digest := sha256.Sum256([]byte(request.CodeVerifier))
		challenge := base64.RawURLEncoding.EncodeToString(digest[:])
		if subtle.ConstantTimeCompare([]byte(challenge), []byte(payload.CodeChallenge)) != 1 {
			return ErrSheJaneInvalidGrant
		}
		user, session, err := model.LockSheJaneAuthorizationIdentityWithTx(tx, flow.UserId, flow.SessionId)
		if err != nil {
			return ErrSheJaneInvalidGrant
		}
		now := time.Now().Unix()
		if user.Status != common.UserStatusEnabled || user.AuthVersion != payload.UserAuthVersion || session.Status != model.UserSessionStatusActive || session.RevokedAt != 0 || session.ExpiresAt <= now || session.Version != payload.SessionVersion || session.UserAuthVersion != payload.UserAuthVersion {
			return ErrSheJaneInvalidGrant
		}
		count, err := model.CountUserTokensWithTx(tx, flow.UserId)
		if err != nil {
			return err
		}
		if count >= int64(operation_setting.GetMaxUserTokens()) {
			return ErrSheJaneTokenLimit
		}
		key, err := common.GenerateKey()
		if err != nil {
			return err
		}
		emptyAllowIPs := ""
		issuedToken = model.Token{
			UserId: flow.UserId, Key: key, Status: common.TokenStatusEnabled,
			Name: sheJaneTokenName(payload.Device.Name), CreatedTime: now, AccessedTime: now,
			ExpiredTime: -1, UnlimitedQuota: true, AllowIps: &emptyAllowIPs,
		}
		device = model.SheJaneDevice{
			UserId: flow.UserId, ClientId: SheJaneClientID, Name: payload.Device.Name,
			Platform: payload.Device.Platform, AppVersion: payload.Device.AppVersion, CreatedAt: now,
		}
		return model.CreateSheJaneManagedCredentialWithTx(tx, &issuedToken, &device)
	})
	if err != nil {
		if errors.Is(err, model.ErrAuthFlowInvalid) || errors.Is(err, model.ErrAuthFlowExpired) || errors.Is(err, model.ErrAuthFlowConsumed) {
			return nil, ErrSheJaneInvalidGrant
		}
		return nil, err
	}
	return &SheJaneTokenExchangeResult{AccessToken: "sk-" + issuedToken.Key, TokenType: "Bearer", Device: device}, nil
}

func ListSheJaneDevices(identity AuthIdentity) ([]model.SheJaneDevice, error) {
	if _, _, err := ValidateLoginSession(identity); err != nil {
		return nil, ErrSheJaneSessionInvalid
	}
	return model.ListSheJaneDevices(identity.UserID)
}

func RevokeSheJaneDevice(identity AuthIdentity, deviceId int64) (*SheJaneDeviceRevocationResult, error) {
	if _, _, err := ValidateLoginSession(identity); err != nil {
		return nil, ErrSheJaneSessionInvalid
	}
	device, token, err := model.GetOwnedSheJaneDeviceAndToken(identity.UserID, deviceId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSheJaneDeviceNotFound
		}
		return nil, err
	}
	if device.RevokedAt != 0 {
		return &SheJaneDeviceRevocationResult{ID: device.Id, Revoked: true, Device: *device, AlreadyRevoked: true}, nil
	}
	if err := model.WriteSheJaneTokenDenyFence(token.Key); err != nil {
		return nil, err
	}
	var revoked *model.SheJaneDevice
	var alreadyRevoked bool
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var revokeErr error
		revoked, alreadyRevoked, revokeErr = model.RevokeSheJaneManagedCredentialWithTx(tx, identity.UserID, deviceId, time.Now().Unix())
		return revokeErr
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSheJaneDeviceNotFound
		}
		return nil, err
	}
	if err := model.WriteSheJaneTokenDenyFence(token.Key); err != nil {
		return nil, err
	}
	return &SheJaneDeviceRevocationResult{ID: revoked.Id, Revoked: true, Device: *revoked, AlreadyRevoked: alreadyRevoked}, nil
}

func mapSheJaneFlowError(err error) error {
	switch {
	case errors.Is(err, model.ErrAuthFlowExpired):
		return ErrSheJaneFlowExpired
	case errors.Is(err, model.ErrAuthFlowInvalid), errors.Is(err, model.ErrAuthFlowConsumed):
		return ErrSheJaneFlowInvalid
	default:
		return err
	}
}

func validateSheJaneAuthorizationStart(request *SheJaneAuthorizationStartRequest) error {
	if request == nil || request.ResponseType != SheJaneResponseTypeCode || request.ClientID != SheJaneClientID || request.CodeChallengeMethod != SheJanePKCEMethodS256 {
		return ErrSheJaneInvalidRequest
	}
	challenge, err := base64.RawURLEncoding.DecodeString(request.CodeChallenge)
	if err != nil || len(request.CodeChallenge) != 43 || len(challenge) != 32 || base64.RawURLEncoding.EncodeToString(challenge) != request.CodeChallenge {
		return ErrSheJaneInvalidRequest
	}
	if !validSheJaneBase64URL(request.State, 43, 128) || !validSheJaneRedirectURI(request.RedirectURI) {
		return ErrSheJaneInvalidRequest
	}
	request.Device.Name = strings.TrimSpace(request.Device.Name)
	if !validSheJaneText(request.Device.Name, 80) {
		return ErrSheJaneInvalidRequest
	}
	switch request.Device.Platform {
	case "macos", "windows", "linux":
	default:
		return ErrSheJaneInvalidRequest
	}
	if !validSheJaneText(request.Device.AppVersion, 32) {
		return ErrSheJaneInvalidRequest
	}
	return nil
}

func validSheJaneBase64URL(value string, minLength, maxLength int) bool {
	if len(value) < minLength || len(value) > maxLength || strings.Contains(value, "=") {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && base64.RawURLEncoding.EncodeToString(decoded) == value
}

func validSheJaneVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || strings.ContainsRune("-._~", r)) {
			return false
		}
	}
	return true
}

func sheJaneTokenName(deviceName string) string {
	name := []rune("SheJane: " + deviceName)
	if len(name) > 50 {
		name = name[:50]
	}
	return string(name)
}

func validSheJaneRedirectURI(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Opaque != "" || parsed.User != nil || parsed.Hostname() != "127.0.0.1" || parsed.Path != SheJaneCallbackPath || parsed.EscapedPath() != SheJaneCallbackPath || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return false
	}
	port, err := strconv.Atoi(parsed.Port())
	return err == nil && port >= 1 && port <= 65535
}

func validSheJaneText(value string, maxRunes int) bool {
	length := utf8.RuneCountInString(value)
	if length < 1 || length > maxRunes {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
