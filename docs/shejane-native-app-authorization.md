# SheJane native-app authorization design

Status: Cloud contract `shejane-native-auth-v1` frozen on 2026-07-29 after P1A/P1B implementation and automated contract verification. Operator-backed pre-release and P1C packaged-app acceptance remain pending.

Repository baseline: `afe16c64cd73853da1eda3bf236f15d69637b4bf` (`main`, 2026-07-28)

Scope: SheJane Cloud authorization only. Billing, telemetry, payment, and SheJane Client implementation are out of scope.

Contract freeze: Sections 3-7 are normative for Runtime integration. The six API endpoints, fixed client ID, exact loopback callback, request/response fields, public error codes, TTLs, and security headers cannot change within `shejane-native-auth-v1`. The official Cloud origin is intentionally not part of this document: Runtime must compile one operator-approved HTTPS origin and must not accept an origin returned by Client, browser, or Cloud. Any incompatible protocol change requires a new contract identifier and parallel rollout; documentation edits cannot silently redefine v1.

## 1. Decision summary

Implement one narrow authorization-code profile for the fixed public client `shejane-desktop`:

- SheJane Runtime opens the system browser with `state` and PKCE S256.
- Cloud validates and persists the native request before login, so login continuation carries only an opaque flow token.
- A live New API dashboard session approves or denies the immutable request.
- Approval creates a two-minute, HMAC-only authorization code bound to the approving user/session, client, redirect URI, PKCE challenge, and device metadata.
- A public, rate-limited form endpoint atomically consumes the code and creates one ordinary New API inference `Token` plus one small `SheJaneDevice` record.
- The raw `sk-...` credential is returned once in the HTTPS token response. It is never put in a browser URL, cookie, log, audit record, or generic token reveal response.
- Device revocation disables and soft-deletes the linked Token and publishes a deny fence to the shared Token cache before reporting success.

`service/shejane_authorization.go` is the single workflow state owner. It orchestrates the existing `User`, `UserSession`, `AuthFlow`, and `Token` models plus the new `SheJaneDevice` model. There is no second account, quota, subscription, balance, or usage ledger.

This is intentionally not a general OAuth provider. Do not add dynamic client registration, a desktop client secret, refresh tokens, scopes, plugins, or provider abstractions in P1.

## 2. Source findings and settled repository questions

### 2.1 Existing authentication and login continuation

Current browser authentication is an in-memory dashboard access token backed by a server-side `UserSession`, not a general cookie-authenticated page session:

- `middleware.UserAuth` accepts either an internal dashboard JWT or a legacy PAT.
- `middleware.GetSessionAuthIdentity` rejects PATs because they have no `SessionID` or `SessionVersion`.
- `service.ValidateLoginSession` checks the session status/expiry/version and the user's enabled status/auth version on every authenticated request.
- The refresh credential is an HttpOnly, SameSite=Strict cookie scoped to `/api/user/auth`; it is not available to `/shejane/authorize` and must not be widened.

The TanStack Router currently preserves `search.redirect` through direct password, passkey, WeChat, and Telegram completion by calling `sanitizeAuthRedirect`, which accepts only same-origin HTTP(S) destinations. It does not preserve the destination through all required paths:

- sign-up has no `redirect` search contract and returns to plain `/sign-in`;
- the 2FA route stores only its `flow_token`, then calls `handleLoginSuccess` without a destination;
- external OAuth performs a full-page round trip, while the current OAuth `AuthFlow` payload stores only affiliate data and the callback normally has no `redirect` value.

P1B therefore adds one small continuation mechanism to `web/src/features/auth/lib/auth-redirect.ts`: save `{path, expires_at}` in tab-scoped `sessionStorage`, only after `sanitizeAuthRedirect`, with the same ten-minute lifetime as the pending Cloud flow. `useAuthRedirect` consumes it after password/passkey/2FA/WeChat/Telegram success; the external OAuth callback consumes it when no valid explicit redirect exists. The value is a same-origin path containing only `flow_token`. Replacing it cannot alter client ID, redirect URI, state, challenge, or device metadata because those are stored server-side before login.

### 2.2 AuthFlow reuse

`model/auth_flow.go` already supplies the right deep module:

- 32 random bytes encoded as an opaque base64url token;
- HMAC-only persistence in the unique `token_hash` column;
- purpose/intent/user/session matching and TTL checks;
- `lockForUpdate` plus a conditional `consumed_at IS NULL` update, which remains single-use on SQLite, MySQL, and PostgreSQL;
- `ConsumeAuthFlowWithAction`, which rolls back consumption when its transactional action fails;
- master-node cleanup of expired/consumed rows, retaining no raw flow token.

Add one purpose, `AuthFlowPurposeSheJaneAppAuthorization`, and two intents, `pending` and `code`. Add `CreateAuthFlowWithTx` so approval can consume the pending flow and create its two-minute code in the same transaction. Do not create another authorization-code table.

The pending flow has a ten-minute TTL to allow registration, login, 2FA, passkeys, and external OAuth. The approved code is a different random value with a two-minute TTL. Separating them prevents a login-continuation token from also being an exchangeable code.

### 2.3 Inference Token semantics

The issued record is a normal New API `Token`, because `middleware.TokenAuth`, `/v1/models`, routing, billing, model limits, logs, and cache behavior already depend on it.

Use these fields:

| Token field | Value | Reason |
|---|---:|---|
| `Status` | enabled | Required by `ValidateUserToken`. |
| `UnlimitedQuota` | `true` | Removes a second token-local wallet. User wallet/subscription quota is still checked and charged by `BillingSession`; “unlimited” applies only to the Token's local cap. |
| `RemainQuota` / `UsedQuota` | `0` initially | Existing settlement updates token usage; no duplicate starting balance is created. |
| `Group` | empty | Inherit the user's current group and usable groups; do not freeze a stale group at authorization time. |
| `ModelLimitsEnabled` | `false` | `/v1/models` and relay routing continue to expose models allowed by the user's current group. A static issuance-time list would drift. |
| `ExpiredTime` | `-1` | P1 has no refresh credential or silent rotation contract. The key lives until explicit device revocation. Add a finite lifetime only together with a designed reauthorization/rotation experience. |
| `AllowIps` | empty | A desktop network address is not stable enough for a useful allowlist. |
| `CrossGroupRetry` | `false` | No new routing privilege. |
| `Name` | `SheJane: <device name>` | Fits the existing 50-character limit after server-side truncation. |

This keeps existing billing behavior: user balance or subscription remains authoritative, and the requested model is not silently changed. It also records a deliberate security ceiling: the result is a long-lived bearer API key accepted by existing inference routes, not a sender-constrained OAuth access token. P1 compensates with TLS, OS credential storage, one-time response, no generic reveal, and immediate device revocation. Audience- or route-constrained Token kinds are a later change only if a concrete requirement justifies modifying shared `TokenAuth`.

### 2.4 Minimum device persistence

Add one table-backed model:

```go
type SheJaneDevice struct {
    Id         int64  `gorm:"primaryKey"`
    UserId     int    `gorm:"not null;index"`
    TokenId    int    `gorm:"not null;uniqueIndex"`
    ClientId   string `gorm:"type:varchar(32);not null"`
    Name       string `gorm:"type:varchar(80);not null"`
    Platform   string `gorm:"type:varchar(16);not null"`
    AppVersion string `gorm:"type:varchar(32);not null"`
    CreatedAt  int64  `gorm:"type:bigint;not null"`
    RevokedAt  int64  `gorm:"type:bigint;not null"`
}
```

`Id` is the Cloud device-connection identity exposed to device management. `TokenId` is one-to-one and is the marker that makes a Token managed/non-revealable. `UserId` keeps ownership checks direct even after the linked Token is soft-deleted. `RevokedAt=0` means active. Do not add last-seen tracking, hardware fingerprints, refresh-token state, per-device quota, or telemetry fields.

The generic Token list may continue to show its already-masked key. The following generic operations must reject an active managed Token with `403 TOKEN_MANAGED_KEY_NOT_REVEALABLE` or `403 TOKEN_MANAGED_EXTERNALLY`:

- `POST /api/token/:id/key`;
- `POST /api/token/batch/keys` if any requested token is managed (no partial result);
- `PUT /api/token`;
- `DELETE /api/token/:id`;
- `POST /api/token/batch` if any requested token is managed.

This closes both disclosure and orphan-device paths. The dedicated device endpoint is the only user-facing mutation path for a managed Token.

## 3. Standards basis

The protocol applies the relevant parts of:

- [RFC 8252, OAuth 2.0 for Native Apps](https://www.rfc-editor.org/rfc/rfc8252.html);
- [RFC 7636, Proof Key for Code Exchange](https://www.rfc-editor.org/rfc/rfc7636.html);
- [RFC 6749, OAuth 2.0 Authorization Framework](https://www.rfc-editor.org/rfc/rfc6749.html);
- [RFC 9700, OAuth 2.0 Security Best Current Practice](https://www.rfc-editor.org/rfc/rfc9700.html).

This one-client profile intentionally supports only IPv4 loopback. It applies the native-app protections but does not claim full general RFC 8252 server support, whose Section 7 covers more redirect patterns. Rejecting IPv6 `::1` is an explicit interoperability narrowing.

Normative consequences:

- use the external system browser, never an embedded credential web view ([RFC 8252 Sections 4-6](https://www.rfc-editor.org/rfc/rfc8252.html#section-5));
- treat the installed app as a public client and never embed a shared client secret ([RFC 8252 Sections 8.4-8.5](https://www.rfc-editor.org/rfc/rfc8252.html#section-8.4));
- require transaction-specific PKCE and exactly `S256`; never downgrade to `plain` ([RFC 7636 Sections 4.2 and 7.2](https://www.rfc-editor.org/rfc/rfc7636.html#section-4.2), [RFC 9700 Section 2.1.1](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1.1));
- accept a verifier only when it is 43-128 characters from `ALPHA / DIGIT / "-" / "." / "_" / "~"`; require the S256 challenge to be the canonical 43-character unpadded base64url encoding of a 32-byte digest ([RFC 7636 Sections 3 and 4.1](https://www.rfc-editor.org/rfc/rfc7636.html#section-4.1));
- allow an ephemeral loopback port but exact-match the persisted URI at exchange; wildcard redirect matching is forbidden ([RFC 8252 Sections 7.3 and 8.4](https://www.rfc-editor.org/rfc/rfc8252.html#section-7.3), [RFC 9700 Section 4.1.3](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.1.3));
- reflect `state` byte-for-byte and require Runtime to compare and delete it before exchange; `state` is correlation/CSRF protection, not authentication ([RFC 6749 Section 10.12](https://www.rfc-editor.org/rfc/rfc6749.html#section-10.12), [RFC 9700 Section 2.1](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1));
- use a short, single-use code bound to client and redirect URI; two minutes is stricter than RFC 6749's recommended ten-minute maximum ([RFC 6749 Sections 4.1.2 and 10.10](https://www.rfc-editor.org/rfc/rfc6749.html#section-4.1.2));
- send the exchange as UTF-8 `application/x-www-form-urlencoded` with `grant_type=authorization_code`, `code`, exact `redirect_uri`, `client_id`, and `code_verifier` ([RFC 6749 Sections 3.2 and 4.1.3](https://www.rfc-editor.org/rfc/rfc6749.html#section-4.1.3), [RFC 7636 Section 4.5](https://www.rfc-editor.org/rfc/rfc7636.html#section-4.5));
- never redirect when the client or redirect URI has not already been validated ([RFC 6749 Section 4.1.2.1](https://www.rfc-editor.org/rfc/rfc6749.html#section-4.1.2.1));
- collapse invalid, expired, replayed, mismatched, or PKCE-failed codes to public `invalid_grant` ([RFC 6749 Section 5.2](https://www.rfc-editor.org/rfc/rfc6749.html#section-5.2));
- put credentials only in an HTTPS response body with `Cache-Control: no-store` and `Pragma: no-cache` ([RFC 6749 Section 5.1](https://www.rfc-editor.org/rfc/rfc6749.html#section-5.1));
- send `Referrer-Policy: no-referrer` on the authorization page and avoid third-party resources while a flow is pending ([RFC 9700 Sections 4.2.4 and 4.3](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.2.4)).

## 4. Complete sequence

### 4.1 Start and login continuation

1. Runtime binds an HTTP listener to `127.0.0.1` on a dynamic port and fixed path `/shejane/auth/callback`.
2. Runtime generates and retains in memory:
   - 32 random bytes encoded as a 43-character base64url `state`;
   - a 43-128 character RFC 7636 verifier;
   - `BASE64URL(SHA256(ASCII(verifier)))` as the 43-character challenge.
3. Client opens the system browser at:

   ```text
   https://<cloud>/shejane/authorize
     ?response_type=code
     &client_id=shejane-desktop
     &redirect_uri=http%3A%2F%2F127.0.0.1%3A<port>%2Fshejane%2Fauth%2Fcallback
     &code_challenge=<challenge>
     &code_challenge_method=S256
     &state=<state>
     &device_name=<display-name>
     &platform=<macos|windows|linux>
     &app_version=<version>
   ```

4. The public page posts those fields to `/api/shejane/authorize/start`. Cloud validates them, writes an `AuthFlow(intent=pending, TTL=10m)`, and returns `flow_token`.
5. The page immediately replaces its URL with `/shejane/authorize?flow_token=<opaque>`; the original redirect/challenge/state/device parameters are no longer used.
6. If no dashboard user is loaded, the page saves the sanitized same-origin continuation in tab `sessionStorage` and navigates to `/sign-in?redirect=<same-origin-flow-path>`.
7. Registration returns to sign-in; password, passkey, WeChat, Telegram, 2FA, and external OAuth all consume the same stored continuation after successful authentication.
8. The authorization page calls the authenticated consent-read endpoint. A PAT receives `AUTH_SESSION_REQUIRED`; only a currently valid browser-backed `UserSession` proceeds.

### 4.2 Consent, approval, and cancellation

1. Consent displays the fixed app name “SheJane Desktop”, device name, platform, app version, the inference-only purpose, and a clear Approve/Cancel choice. Device metadata is escaped text and never HTML.
2. The read response deliberately omits `redirect_uri`, `state`, code challenge, and any key.
3. On Approve, the controller rechecks `GetSessionAuthIdentity`. In one main-database transaction it:
   - locks and consumes the pending flow;
   - creates a new `AuthFlow(intent=code, TTL=2m)` with the immutable payload;
   - binds the code row to user ID and session ID;
   - records the current user auth version and session version in the typed payload.
4. The response contains only `redirect_to`, built from the stored validated loopback URI with `code=<new opaque code>&state=<original state>`. The SPA performs `window.location.replace(redirect_to)`.
5. On Cancel, Cloud consumes the pending flow and returns the stored callback with `error=access_denied&state=<original state>`. No code or Token is created.
6. An invalid client/redirect/start request is rendered on Cloud and never redirected. An invalid or expired opaque flow that cannot yield a trusted persisted callback is also rendered on Cloud.
7. A transient internal failure before a terminal decision leaves the pending flow unconsumed and returns a same-origin generic error so the user can retry. Internal details never enter the loopback URL.

### 4.3 Exchange and Runtime completion

1. Runtime first exact-compares callback `state` with its in-memory value and deletes the saved value. Missing/mismatch stops the flow without calling Cloud.
2. Runtime sends an HTTPS form POST to `/api/shejane/token` with the five required fields.
3. Inside `ConsumeAuthFlowWithAction`, Cloud:
   - locks the code row and verifies `intent=code`, TTL, and unused state;
   - parses the typed payload with `common.UnmarshalJsonStr`;
   - constant-time compares the recomputed S256 challenge;
   - exact-matches fixed client ID and persisted redirect URI;
   - locks and revalidates the bound `User` and `UserSession`, including enabled/status, expiry, user auth version, and session version;
   - locks the user row, enforces the existing maximum Token count, generates a normal New API key, and inserts the Token and `SheJaneDevice` row.
4. Code consumption, Token insert, and device insert commit together. Any validation or insert error rolls back all three.
5. Cloud returns `sk-<stored Token.Key>` once. If the network response is lost after commit, the code remains consumed and the key is not recoverable; the user revokes that device record and starts again.
6. Runtime stores the key in the operating-system credential store, closes the loopback listener, uses the compiled immutable SheJane Cloud base URL, calls existing `/v1/models`, and runs existing capability verification.

## 5. Endpoint contracts and middleware

Every `/api` route already receives `RouteTag("api")`, gzip, body-storage cleanup, and `GlobalAPIRateLimit`. Do not add `middleware.CORS()` to these routes. The browser endpoints are same-origin; the native token exchange does not need browser CORS.

| Endpoint | Route middleware after the global chain | Authentication | Purpose |
|---|---|---|---|
| `GET /shejane/authorize` | `GlobalWebRateLimit`; special `no-store`, `Pragma: no-cache`, `Referrer-Policy: no-referrer` response headers | Public SPA page | Parse start query or resume by opaque `flow_token`. |
| `POST /api/shejane/authorize/start` | `CriticalRateLimit`, `DisableCache`, `AnonymousRequestBodyLimit` | Public | Validate and persist immutable pending request. |
| `GET /api/shejane/authorize/:flow_token` | `UserAuth`, `DisableCache` | Controller additionally requires `GetSessionAuthIdentity` | Return consent-safe display data. |
| `POST /api/shejane/authorize/:flow_token` | `UserAuth`, `SessionCookieOriginGuard`, `CriticalRateLimit`, `DisableCache`, `AnonymousRequestBodyLimit` | Live browser session only | Approve or deny and return trusted loopback destination. |
| `POST /api/shejane/token` | `CriticalRateLimit`, `DisableCache`, `AnonymousRequestBodyLimit` | Public | Consume code + PKCE and return raw inference key once. |
| `GET /api/shejane/devices` | `UserAuth`, `DisableCache` | Live browser session only | List device records without Token IDs or keys. |
| `DELETE /api/shejane/devices/:id` | `UserAuth`, `SessionCookieOriginGuard`, `CriticalRateLimit`, `DisableCache` | Live browser session only | Idempotently revoke device and Token. |

`SessionCookieOriginGuard` is reused on browser security mutations even though `UserAuth` uses an Authorization header. Production deployment must retain the existing secure-cookie/origin configuration. The middleware is not applied to the native token exchange.

### 5.1 Start authorization

Request (`application/json`):

```json
{
  "response_type": "code",
  "client_id": "shejane-desktop",
  "redirect_uri": "http://127.0.0.1:49152/shejane/auth/callback",
  "code_challenge": "43-character-unpadded-base64url",
  "code_challenge_method": "S256",
  "state": "43-to-128-character-base64url",
  "device": {
    "name": "Jimmy's Mac",
    "platform": "macos",
    "app_version": "0.1.8"
  }
}
```

Validation is server-side and exact:

- only `response_type=code` and `client_id=shejane-desktop`;
- only `code_challenge_method=S256` and canonical 43-character base64url challenge decoding to 32 bytes;
- `state` must be 43-128 unpadded base64url characters;
- `redirect_uri` must parse to scheme `http`, hostname text exactly `127.0.0.1`, explicit numeric port 1-65535, path exactly `/shejane/auth/callback`, no userinfo, fragment, query, wildcard, or empty/trailing alternate path;
- device name 1-80 characters after trimming, with control characters rejected; platform is one of `macos`, `windows`, `linux`; app version is 1-32 printable characters.

Success (`201`):

```json
{
  "success": true,
  "message": "",
  "data": {
    "flow_token": "opaque-pending-token",
    "expires_at": 1785233400
  }
}
```

Malformed or unsupported input (`400`) is never redirected:

```json
{"success":false,"code":"SHEJANE_INVALID_REQUEST","message":"invalid authorization request"}
```

### 5.2 Read consent

Success (`200`):

```json
{
  "success": true,
  "message": "",
  "data": {
    "client": {"id":"shejane-desktop","name":"SheJane Desktop"},
    "device": {"name":"Jimmy's Mac","platform":"macos","app_version":"0.1.8"},
    "expires_at": 1785233400
  }
}
```

Errors: `401` for invalid/expired dashboard JWT, `403 AUTH_SESSION_REQUIRED` for PAT, `404 SHEJANE_FLOW_INVALID` for unknown/consumed flow, and `410 SHEJANE_FLOW_EXPIRED` for expiry. No response contains callback, state, challenge, code, verifier, Token ID, or key.

### 5.3 Approve or deny

Request (`application/json`):

```json
{"decision":"approve"}
```

`decision` is exactly `approve` or `deny`. Success for approval (`200`):

```json
{
  "success": true,
  "message": "",
  "data": {
    "redirect_to": "http://127.0.0.1:49152/shejane/auth/callback?code=opaque-code&state=original-state"
  }
}
```

Success for denial uses the same envelope with `error=access_denied&state=original-state`. Errors use the consent status/code set above plus `400 SHEJANE_INVALID_DECISION` and generic `500 SHEJANE_INTERNAL_ERROR`. The endpoint never reflects a callback supplied in this request.

### 5.4 Exchange code

Request:

```http
POST /api/shejane/token
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&code=...&redirect_uri=...&client_id=shejane-desktop&code_verifier=...
```

Success (`200`, always `Cache-Control: no-store, no-cache...` and `Pragma: no-cache`):

```json
{
  "access_token": "sk-raw-key-returned-once",
  "token_type": "Bearer"
}
```

Errors use the OAuth shape, not the dashboard envelope:

| Status | Body | Applies to |
|---:|---|---|
| `400` | `{"error":"invalid_request"}` | wrong content type/grant type, missing field, syntactically invalid verifier/URI |
| `400` | `{"error":"invalid_grant"}` | unsupported/mismatched client, unknown, expired, consumed, replayed, redirect-mismatched, PKCE-mismatched, or no-longer-live user/session |
| `429` | empty body plus existing `Retry-After` | `CriticalRateLimit` rejection |
| `500` | `{"error":"server_error"}` | database/key-generation failure |

All exchange errors also receive no-store/no-cache headers. No error description includes a code, verifier, challenge, redirect, key, user ID, or session ID.

### 5.5 List and revoke devices

List success (`200`):

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 17,
      "client_id": "shejane-desktop",
      "name": "Jimmy's Mac",
      "platform": "macos",
      "app_version": "0.1.8",
      "created_at": 1785232800,
      "revoked_at": 0
    }
  ]
}
```

No device response includes `token_id`, key material, state, challenge, session ID, IP, or browser user agent.

Revocation success, including repeat revocation (`200`):

```json
{"success":true,"message":"","data":{"id":17,"revoked":true}}
```

An unknown or other-user ID returns `404 SHEJANE_DEVICE_NOT_FOUND`, never `403`, to avoid an ownership oracle.

## 6. Transaction and cache boundaries

### 6.1 Approval

`ConsumeAuthFlowWithAction(pending)` and `CreateAuthFlowWithTx(code)` share one main-database transaction. A code-row insert failure rolls back pending consumption. The browser receives a callback only after commit.

### 6.2 Exchange

One main-database transaction owns the security state change:

```text
lock code -> validate payload/PKCE -> lock user/session ->
check Token limit -> consume code -> insert Token -> insert SheJaneDevice -> commit
```

The existing conditional `consumed_at IS NULL` update is the cross-database replay gate. `lockForUpdate` emits `FOR UPDATE` on MySQL/PostgreSQL and skips unsupported syntax on SQLite; the conditional update still permits exactly one successful consumer. No Redis operation is needed on issuance because the key is new.

### 6.3 Revocation

The current `Token.Delete` deletes Redis asynchronously after committing the database row. A delayed pre-revocation cache fill can repopulate an enabled Token, so that path does not meet this design.

Reuse the stronger pattern already present in `model/user_session.go`:

1. Load the owned device and linked Token, including an already-revoked record for idempotency.
2. If Redis is enabled, synchronously write a short-lived managed-token deny fence keyed by the existing HMAC cache key. If this write fails, do not mutate the database and return a retryable server error.
3. In one main-database transaction, lock device and Token, set `revoked_at`, set Token status disabled, and soft-delete the Token.
4. Commit, then refresh the deny fence. A successful response means both authoritative rows and the shared cache deny the credential.
5. `cacheGetTokenByKey` treats the fence as disabled without database fallback, and asynchronous `cacheSetToken` refuses to overwrite it with an older enabled snapshot.

If the database commit fails after the pre-fence, the credential can be temporarily denied until the bounded fence expires even though revocation returned an error. This conservative availability cost is preferable to a successful-looking revocation that leaves a usable credential. If Redis is disabled or unavailable after a committed revocation, Token lookup falls through to the soft-deleted database row and fails.

The fence is a focused change in `model/token_cache.go`, not a new cache system.

## 7. Threat model and invariants

### 7.1 Assets, actors, and seams

Assets: Cloud account/session, user quota/subscription, authorization code, inference Token, provider channel keys, and future telemetry credentials.

Attacker-controlled inputs: every native request field, loopback port, device metadata, flow/code/verifier, HTTP headers, client IP, and numeric device ID. Operator-controlled inputs include Cloud origin, session-cookie configuration, rate-limit settings, database, Redis, model channels, and user/group policy. Developer-controlled inputs include the fixed client/callback constants and Runtime's compiled Cloud base URL.

Trust seams:

- Runtime process ↔ system browser;
- browser SPA ↔ dashboard access token/live `UserSession`;
- public browser/native requests ↔ Cloud validation;
- Cloud workflow module ↔ main database transaction;
- authoritative Token row ↔ shared Redis Token cache;
- inference Token ↔ upstream routing/billing;
- operating-system credential store ↔ local Runtime.

### 7.2 Required invariants and prevention

| Threat | Prevention/proof |
|---|---|
| Authorization-code interception | Exact loopback template plus S256; an intercepted code is unusable without Runtime's verifier. |
| PKCE downgrade | Start accepts only literal `S256`; method is stored with the code and is never chosen from exchange input. |
| Code replay/concurrent exchange | HMAC-only random code, two-minute TTL, row lock, and conditional single-use update; Token/device creation shares that transaction. |
| Redirect substitution | Full URI is strictly validated once, stored in pending/code payloads, exact-matched at exchange, and never accepted from consent. |
| Login-continuation substitution | Continuation carries only a sanitized same-origin `flow_token`; all security fields are immutable server-side. Consent re-displays the stored app/device. |
| Browser session/PAT substitution | Approval, device list, and revoke call `GetSessionAuthIdentity`; code binds user/session/auth/session versions; exchange revalidates locked user/session rows. |
| User disable/logout during flow | Exchange rejects changed auth/session versions, disabled user, inactive/expired/revoked session with generic `invalid_grant`. |
| Token disclosure | No key in URLs/cookies/logs/audit; no-store/no-cache; key appears only in successful exchange; managed marker blocks individual/batch generic reveal. |
| Partial exchange failure | Code consume + Token insert + device insert are one DB transaction; action error rolls back consumption. |
| Partial revocation/cache race | Deny fence precedes DB mutation; old async fills cannot overwrite it; successful response follows DB commit. |
| CSRF on approval/revoke | Live bearer-backed session, same-origin SPA, no API CORS, `SessionCookieOriginGuard`, `state`, and visible consent. |
| Open redirect/error leakage | Invalid client/redirect never redirects; terminal errors use only persisted validated URI and opaque allowlisted error codes. |
| Device-ID enumeration | Ownership is always part of the query; missing and other-user records both return 404. |
| Resource exhaustion | Global API limit + critical IP limit + anonymous body limit + strict field lengths + existing per-user Token maximum. |
| XSS/referrer leakage | React text escaping, no third-party consent resources, no-referrer header, and immediate URL replacement to opaque flow token. |
| Cross-purpose credential use | Only a New API inference Token is issued. No dashboard JWT/PAT, refresh cookie, upstream provider key, or telemetry credential is created or accepted. |

`state` must be high entropy and one-time in Runtime, but Cloud never treats it as authentication. Device name/platform/version are display metadata, never authorization facts.

### 7.3 Logging and audit

- Request logs may record method/path/status/request ID/IP, but must not record request/response bodies, Authorization headers, form fields, raw query codes, verifier, or key.
- All public validation failures use stable reason classes. Server logs may include request ID and coarse class, never secret values.
- After successful exchange, call `recordUserSecurityAudit` with action `user.shejane_device_authorized` and only device record ID, fixed client ID, platform, and app version.
- After successful revoke, record `user.shejane_device_revoked` with the same non-secret identifiers and whether it was already revoked.
- Audit uses `LOG_DB` after the main transaction and is best effort, matching current operation-audit behavior. An audit write failure is logged server-side but cannot roll back an already-issued/revoked credential across a separate log database.
- Do not audit the raw state, flow/code hash, redirect URI, session ID, Token ID/key, or device name.

### 7.4 Cleanup and retained data

The existing master-node auth cleanup job removes flows after expiry/consumption retention. Raw pending/code values are never stored; only HMAC plus non-secret payload remains. Keep the existing 24-hour cleanup retention for diagnosis, then delete the row.

Active device metadata remains until revocation. Revoked device rows are retained for account security history; the linked Token remains only as a soft-deleted disabled row under existing Token retention. A later privacy-retention policy may purge old revoked device metadata, but P1 does not invent one.

## 8. Exact implementation surface

### 8.1 Backend production files

| File | Minimum change |
|---|---|
| `model/auth_flow.go` | Add SheJane purpose/intents and `CreateAuthFlowWithTx`; keep raw-token generation/HMAC/consume logic centralized. |
| `model/shejane_device.go` | New model and transaction-aware ownership/list/managed-token/revoke operations. |
| `model/token_cache.go` | Add managed-token deny fence and prevent stale enabled snapshots from overwriting it. |
| `model/main.go` | Add `SheJaneDevice` to normal and fast additive AutoMigrate lists. |
| `service/shejane_authorization.go` | Single workflow module: validate/start, consent view, decision, exchange, list, revoke. Typed AuthFlow payload lives here. |
| `controller/shejane_authorization.go` | Parse exact JSON/form contracts, require browser session where specified, map only stable public errors, emit audits. |
| `controller/token.go` | Reject managed Token reveal/update/delete in individual and batch paths. |
| `router/api-router.go` | Register the six API routes with the exact middleware order in Section 5. |
| `router/web-router.go` | Apply no-store/no-referrer headers to embedded `/shejane/authorize`; external frontend hosting must mirror them. |

No change is required in relay routing, billing, user/account models, provider adapters, or `relaykit/`.

### 8.2 Frontend production files

| File/module | Minimum change |
|---|---|
| `web/src/routes/shejane/authorize.tsx` | Public, Zod-validated route for start/resume and consent. |
| `web/src/features/shejane-authorization/` | Small API/types/view module for consent and decision; no new state library. |
| `web/src/features/auth/lib/auth-redirect.ts` | Add sanitized, ten-minute, tab-scoped save/consume helpers. |
| `web/src/features/auth/hooks/use-auth-redirect.ts` | Consume saved continuation when no valid explicit destination exists. |
| `web/src/routes/oauth/$provider.tsx` | Use saved continuation after successful external OAuth when no valid callback redirect exists. |
| `web/src/features/profile/components/shejane-devices-card.tsx` and profile composition | List and revoke device connections without showing keys. |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json` | Consent/device/error text via existing i18n sync. |

The Client/Runtime repository is untouched until P1C. Its later fixed callback path and Cloud base URL must match this contract exactly.

## 9. Focused tests

Use `testify/require` for setup/fatal assertions and `testify/assert` for values. Tests are deterministic; no sleeps, random stress loops, or implementation-detail snapshots.

### 9.1 Model/service tests

1. Redirect/PKCE table: valid IPv4 loopback dynamic ports; reject `plain`, missing/unknown method, padded/non-canonical challenge, short/long/invalid verifier, `localhost`, IPv6, remote hosts, userinfo, fragments, queries, alternate paths, schemes, and redirect mismatch.
2. Pending/code lifecycle: start, ten-minute pending semantics, approve creates a distinct two-minute code, deny consumes without code, expired/consumed/replayed flows fail.
3. Session binding: PAT rejected; wrong user/session/version, revoked/expired session, disabled user, and changed auth version all fail.
4. Concurrent exchange: two deterministic callers of the same code produce exactly one Token/device and one success.
5. Atomic rollback: injected Token insert and device insert failures leave the code unconsumed and no orphan Token/device; successful retry works.
6. Token fields: issued Token is enabled, unlimited only at token-local level, group empty, model limits disabled, no expiry, no IP restrictions.
7. Token limit: at the configured maximum, exchange fails and code remains unconsumed; serialized same-user exchanges cannot exceed the limit.
8. Managed disclosure: individual/batch key reveal, update, and generic delete reject managed Tokens; normal personal Tokens retain current behavior.
9. Revocation: owner succeeds, other user gets 404, repeat is success, Token is disabled/soft-deleted, and a cached credential is immediately rejected.
10. Cache race: a pre-revocation enabled snapshot cannot overwrite the deny fence; Redis fence failure leaves DB state active and returns failure.

### 9.2 Controller/router tests

1. Exact success and error JSON/form contracts and status codes from Section 5.
2. Every exchange response, including errors, has no-store/no-cache headers; raw key occurs only in the one successful exchange body.
3. Device list/consent and audit payloads contain none of key, Token ID, state, challenge, verifier, redirect, or session ID.
4. Invalid redirect/client never yields `redirect_to`; denial uses only the stored exact callback and original state.
5. Route-level PAT/live-session behavior, OriginGuard rejection, anonymous body limit, and critical-rate `429`/`Retry-After` behavior.

### 9.3 Frontend tests

1. Start fields are sent once, the URL is replaced with opaque `flow_token`, and unauthenticated navigation stores only a sanitized same-origin continuation.
2. Saved continuation survives sign-up, 2FA, and external OAuth completion, expires after ten minutes, is consumed once, and rejects external/ambiguous paths.
3. Consent displays stored app/device metadata, requires an explicit decision, and top-level navigates only to server-returned loopback `redirect_to`.
4. Cancel uses the error callback; invalid/expired flow remains on a safe Cloud error surface.
5. Device list never renders a key and revoke requires confirmation, handles repeat success, ownership-safe 404, loading, and failure states accessibly.

## 10. Rollout, rollback, and estimate

### 10.1 Implementation order

1. P1A tests for validators, AuthFlow transactions, Token disclosure, and cache fence.
2. Additive device migration and backend workflow module.
3. Controllers/routes and exact HTTP contract tests.
4. P1B consent route plus centralized login continuation.
5. Device-management card, i18n, accessibility, and focused frontend tests.
6. Deploy backend first with no caller, then frontend; canary with a test user across password, 2FA, passkey, external OAuth, cancellation, response loss, replay, and Redis-backed revocation.
7. Freeze the Cloud contract before starting P1C in the separate SheJane repository.

Rollback removes/hides frontend entry points and stops Runtime rollout first. Backend routes can be disabled while the additive `she_jane_devices` table and new AuthFlow purpose remain harmless for upstream mergeability. Existing BYOK/local-first behavior is unaffected.

### 10.2 Verification gates

- focused Go model/service/controller tests;
- full normal backend verification required by the repository;
- SQLite tests plus dialect-safe GORM review for MySQL 5.7.8+ and PostgreSQL 9.6+;
- `bun run i18n:sync`, affected frontend tests, `bun run typecheck`, affected-file lint, and `bun run build`;
- manual system-browser loopback test and immediate revocation test with Redis enabled;
- `cd relaykit && GOWORK=off go build ./...` is not required unless implementation unexpectedly touches `relaykit/` (this design says it must not).

### 10.3 Estimate

For one engineer familiar with this repository:

- P1A backend and focused security tests: 3-4 engineering days;
- P1B consent/device UI, login continuation, i18n, and frontend tests: 2-3 engineering days;
- canary/deployment validation and fixes: 1 day.

Cloud total: **6-8 engineering days**. P1C SheJane Runtime/Client integration is separate and estimated at 2-3 additional days after this contract is stable. Billing and telemetry are not included.

## 11. Explicitly deferred

- dynamic clients, scopes, client secrets, refresh tokens, generalized OAuth discovery, and plugins;
- hardware fingerprinting, last-seen telemetry, per-device quota, or a duplicate balance ledger;
- sender-constrained/audience-constrained inference Token kinds;
- payment, recharge, LangSmith ingestion, crash reporting, or any telemetry credential;
- SheJane Runtime/Client changes, immutable Cloud base URL enforcement, OS credential storage, and capability verification wiring (P1C).

These are deferred because the current secure flow does not require them. Add one only when a concrete product/security requirement exceeds the fixed-client, explicit-revocation profile above.
