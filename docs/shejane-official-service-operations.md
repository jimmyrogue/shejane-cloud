# SheJane official service pre-release operations

Status: the invitation environment for `shejane-native-auth-v1` is live at `https://app.shejane.com`; inference upstream, centralized diagnostics credentials, payment, and public release remain subject to the gates below.

This runbook is subordinate to `docs/shejane-native-app-authorization.md`,
`docs/shejane-paid-operations.md`, and `docs/authentication.md`. It adds no protocol fields and stores no
secrets.

## 1. Release boundary

The P1 Cloud release contains the fixed-client native authorization backend, authorization/device pages, and invitation-only registration control. SheJane Runtime/Client must use contract `shejane-native-auth-v1`. P2 centralized diagnostics follows the separate `docs/shejane-central-diagnostics.md` contract. Billing, payment, and a general OAuth provider remain outside P1.

Never expose an inference Token in a URL, browser page, log, audit payload, support ticket, or deployment output. Never put database, Redis, session, channel, or provider credentials in this repository.

The 2026-07-29 deployment uses PostgreSQL, private Redis, Cloudflare Full (strict), an exact-host Origin certificate, secure cookies, invitation-only password registration, and Passkey bound to `app.shejane.com`. `admin.shejane.com` redirects to the same path on `app.shejane.com`; the retired app, admin, and API containers are stopped while the PostgreSQL container remains in service. Initial quota and invitation rewards remain zero, and no inference upstream is configured.

Public acceptance passed invitation registration, password login, Runtime-owned dynamic loopback and PKCE, approval, denial, local timeout, code replay, an unread successful exchange body, credential retrieval from a fresh Runtime process, and immediate 401 after device revocation. Separate runs verified that 2FA and a Chrome virtual platform authenticator Passkey login continue the same pending authorization. The temporary accounts, devices, and local credential entry were removed after the runs. This evidence does not cover Windows packaging, Developer ID/notarization, a physical Passkey, external OAuth, a legal upstream, or a LangSmith service key.

P3 preparation adds dormant `she_jane_balance_entries`, `she_jane_billing_reservations`, and
`she_jane_payment_events` tables. It adds no payment route and is not a checkout activation. Keep every
generic payment method unconfigured for the invitation beta unless the separate paid-operation activation
gate has been approved in full.

## 2. Required topology

- One PostgreSQL primary database. PostgreSQL 15 is the tested Compose baseline; the application remains compatible with PostgreSQL 9.6+.
- One shared Redis for all Cloud nodes. The Token deny fence and shared critical rate limits require every node to use the same Redis.
- One HTTPS reverse proxy in front of the application. Forward only from explicitly configured proxy IP/CIDR values.
- `https://app.shejane.com` is the only browser and native-authorization origin. `https://admin.shejane.com` redirects to the same path on `app.shejane.com`; it is not a second session or API origin.
- One operator-approved, legally authorized upstream channel with only the models intended for the beta group.
- Backups for PostgreSQL and deployment configuration before the first migration.

The repository `docker-compose.yml` is a local example with placeholder passwords and a generic image. Do not deploy it unchanged. A release environment must inject values through its secret manager and must deploy an image built from the reviewed release revision.

## 3. Environment checklist

Set these outside Git:

| Setting | Required production value |
|---|---|
| `SQL_DSN` | PostgreSQL DSN from the deployment secret manager. |
| `REDIS_CONN_STRING` | Shared Redis DSN from the deployment secret manager. |
| `SESSION_SECRET` | High-entropy value identical on every application node. |
| `CRYPTO_SECRET` | High-entropy value identical on every node sharing Redis; set explicitly rather than relying on fallback. |
| `SESSION_COOKIE_SECURE` | `true`. |
| `SESSION_COOKIE_TRUSTED_URL` | `https://app.shejane.com`. |
| `TRUSTED_PROXIES` | Only the actual reverse-proxy IP/CIDR values. |
| `GENERATE_DEFAULT_TOKEN` | `false`; the native flow creates its own managed inference Token. |
| `BATCH_UPDATE_ENABLED` | Must be `false` on every node before any P3 paid wallet is opened; invitation-only operation may leave P3 dormant. |
| `SHEJANE_LANGSMITH_API_KEY` | LangSmith production service key from the deployment secret manager; never a personal key. |
| `SHEJANE_LANGSMITH_ENDPOINT` | Exact operator-approved regional HTTPS origin, with no path, query, fragment, or userinfo. |
| `SHEJANE_LANGSMITH_PROJECT` | Operator-approved project name for metadata-only SheJane diagnostics Runs. |
| `SHEJANE_LANGSMITH_WORKSPACE_ID` | Required only when the service key can address multiple workspaces. |

Keep TLS verification enabled. Do not set `TLS_INSECURE_SKIP_VERIFY=true`.

## 4. Account and quota checklist

Before inviting testers, set these persisted options in the root settings UI:

| Option | Beta value |
|---|---|
| `RegisterEnabled` | `true`. |
| `PasswordRegisterEnabled` | Operator choice; if enabled, the same invitation rule applies. |
| `InviteRegisterEnabled` | `true`. Missing or invalid affiliate codes reject password, external OAuth, and new WeChat account creation; existing accounts can still sign in. |
| `QuotaForNewUser` | Operator-approved initial inference quota. No value is guessed here. |
| `QuotaForInviter` | `0` until compliance and financial rules explicitly permit rewards. |
| `QuotaForInvitee` | `0` until compliance and financial rules explicitly permit rewards. |

Generate invitation links from existing user affiliate codes and distribute them out of band. Treat the tester list as personal data. Revoking registration globally (`RegisterEnabled=false`) does not revoke existing sessions or SheJane devices.

## 5. Upstream readiness

The operator must record, outside Git:

1. provider identity and the agreement that authorizes this use;
2. channel owner, credential rotation procedure, allowed models, and beta spending ceiling;
3. successful channel test and `/v1/models` visibility for the beta group;
4. quota and rate-limit policy that fails closed when user quota is exhausted;
5. incident contact and upstream-disable procedure.

An upstream being technically reachable is not evidence of authorization. Do not enable a channel until the operator has made and recorded that determination.

## 6. Release sequence

1. Record the reviewed application revision and build immutable backend/frontend artifacts from it.
2. Back up PostgreSQL and verify restore access.
3. Deploy the backend first. Let additive migrations create `she_jane_devices`, `she_jane_telemetry_tokens`,
   the three dormant P3 accounting tables, and the AuthFlow additions, then verify `/api/status` without
   logging secrets. Do not backfill/open wallets or grant the application new journal mutation routes.
   Verify `users.quota_version` is initialized and the balance journal rejects `UPDATE`/`DELETE` through
   both the application database role and its installed database triggers.
4. Deploy the frontend and verify the authorization page carries `Cache-Control: no-store` and `Referrer-Policy: no-referrer`.
5. Set the account/quota options above and configure one authorized upstream channel.
6. Run a canary with one invited test account: approve, deny, expiry, replay, response loss followed by device revocation and a new authorization, device list, and Redis-backed immediate revocation.
7. Only after Cloud canary success, distribute a Runtime build compiled with the approved HTTPS Cloud origin.
8. If centralized diagnostics is in the beta scope, verify explicit opt-in, failure-only default sampling,
   strict metadata, `st-` revocation with the device, and a LangSmith outage that leaves the local Run unchanged.

Repository verification before artifact creation:

```bash
make test
go build ./...
go vet ./model ./service ./controller ./router
cd web
bun install --frozen-lockfile
bun test --max-concurrency 1
bun run i18n:sync
bun run typecheck
bun run build
```

## 7. Rollback

1. Stop distributing or disable the Runtime official-service entry first; BYOK remains available.
2. Remove or hide the Cloud authorization UI entry, then disable the six SheJane API routes at the edge or by rolling back the application artifact.
3. Revoke issued devices through the device API/UI before removing route access when credential invalidation is required.
4. Roll back application artifacts. Keep the additive SheJane device, telemetry, balance-journal,
   reservation, payment-event tables and AuthFlow rows; dropping them is unnecessary and makes recovery
   harder. Never edit or delete journal entries as a rollback mechanism.
5. Restore PostgreSQL only for confirmed data corruption. A normal application rollback must not overwrite newer user, quota, Token, or audit data.
6. Preserve logs that contain request IDs and coarse errors only; rotate any secret suspected of exposure.

## 8. Support and privacy boundary

Support may collect the release revision, platform, coarse timestamp, request ID, device record ID, HTTP
status, and stable error code. Do not request or accept an inference Token, authorization code, state,
PKCE verifier/challenge, redirect URI, browser Session ID, prompt, model output, or local file. If an
exchange response is lost after commit, the credential cannot be recovered: revoke the resulting device
record and start a new authorization. If credential exposure is suspected, revoke the device first and
confirm the old Token returns 401 through both a warm and cold Token-cache path.

Active device metadata and revoked security history follow the retention boundary in the protocol truth.
Do not export the invited tester list into application logs or this repository. Privacy policy, terms,
retention periods, data-subject handling, and jurisdiction-specific conclusions require operator and legal
approval before invitations are sent.

## 9. External decisions still required

- reverse-proxy addresses and Cloudflare-to-origin trust boundary;
- production PostgreSQL/Redis hosts, backup owner, and secret rotation owner;
- exact initial quota, beta user cap, request limits, and spending ceiling;
- invited tester list and support contact;
- authorized upstream provider, models, credentials, and contractual record;
- privacy policy, terms, retention periods, and jurisdiction-specific legal review;
- release window, rollback authority, and packaged macOS/Windows acceptance devices.
- LangSmith region, workspace, project, service key owner, retention, sampling policy, privacy notice, and support access.
- independent crash-reporting vendor, endpoint, platform SDKs, sampling, retention, and user notice.
- paid-service provider/account, currency/minor-unit rule, immutable price catalog, resale authorization,
  opening reconciliation, refund/tax/support/retention/fraud policies, balance/spend/rate limits, webhook
  fixtures and keys, alert destinations, and explicit `BillingSession` cutover approval.

These decisions block production rollout, not further local Runtime/Client implementation or automated testing.
