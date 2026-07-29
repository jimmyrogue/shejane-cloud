# SheJane paid operations boundary

Status: P3 preparation contract. Production charging remains disabled until the external decisions in
this document are recorded by the operator. This contract does not change native authorization or the
central diagnostics protocol.

## Existing primitives remain authoritative

- `User.Quota` remains the hot-path wallet projection and `BillingSession` remains the only request
  reserve/settle/refund owner. SheJane does not add a second spendable wallet.
- `SubscriptionPreConsumeRecord`, Token quota, model pricing, group ratios, quota saturation handling,
  usage logs, provider routing, and the existing dashboard are reused.
- New P3 records are accounting evidence around those primitives: an append-only balance journal, one
  request reservation row, and a verified-webhook audit row.
- Until cutover is explicitly enabled, none of the new records can credit a user, start checkout, or
  alter inference billing. Invitation quota and BYOK continue to work unchanged.

## Activation gate

Paid operation is fail-closed. Activation requires all of the following, and absence of any item keeps
checkout and paid credits unavailable:

1. the repository's current payment-compliance acknowledgement;
2. one named payment provider and its complete server-side configuration;
3. an approved currency and integer minor-unit conversion rule;
4. an approved immutable pricing snapshot/version and upstream resale authorization;
5. refund, tax/invoice, support, retention, fraud, per-user balance, spend, and request-rate policies;
6. a reconciled opening journal entry for every wallet enabled for paid operation.
7. `BATCH_UPDATE_ENABLED` disabled consistently on every Cloud node before any wallet is opened.

No default domain, provider, currency, price, legal conclusion, credential, or monetary limit is inferred.

## Immutable balance journal

`she_jane_balance_entries` is append-only. Each entry contains a user, signed quota delta, balance-after
projection, bounded kind, immutable idempotency key, reference type/id, optional exact price snapshot,
and creation time. Raw payment payloads, signatures, provider secrets, model requests, and inference or
diagnostics credentials are forbidden.

The preparation schema records an immutable `pricing_version` and SHA-256 identity for the exact snapshot,
not an arbitrary JSON blob. The approved snapshot schema and immutable price-catalog storage are still an
activation decision; before cutover, the stored version/hash must resolve to that catalog or the request
fails closed.

The first enabled entry is `opening`; its `balance_after` must equal the locked `User.Quota` projection.
Every later mutation locks that user, applies the quota delta and appends the matching entry in one
database transaction. SQLite, MySQL, and PostgreSQL migrations install `BEFORE UPDATE/DELETE` rejection
triggers; GORM hooks reject ordinary application writes as a second guard. Production database roles must
also deny `UPDATE` and `DELETE` on the journal table.

Reconciliation checks, per user:

```text
latest journal balance_after == users.quota
opening amount + sum(later signed amounts) == latest balance_after
```

A mismatch disables paid mutations for that user and raises an operator alert; it is never repaired by
silently editing history. Correction is a new bounded `adjustment` entry with an operator reference.

Every paid quota mutation also advances a database `quota_version`. Before the transaction changes quota,
Cloud publishes a Redis deny fence for that version; after commit it publishes the exact quota/version
snapshot. Cached readers reject versions below the pending or committed floor, so a delayed database
fallback cannot restore pre-reservation quota. Fence publication failure aborts the mutation, and the
pending fence outlives every old user-cache entry if post-commit publication temporarily fails.
Once an opening entry commits, a durable user flag makes quota reads bypass Redis and use the database
projection. This intentionally conservative beta boundary also prevents legacy top-up, redemption,
check-in, administrator, or batch quota writers from racing a paid snapshot under the same version: a
database trigger rejects any managed-wallet quota change that does not advance `quota_version` in the same
statement. Those legacy operations therefore fail closed until they use the journal owner. A fully
versioned replacement for every legacy quota writer is required before paid-wallet quota caching can be
re-enabled. When legacy batch mode is enabled, user-quota changes use one synchronous conditional database
update and are never queued; only usage/request counters remain batched. A concurrent wallet opening
therefore either captures the committed legacy delta or makes that delta fail before a request proceeds
upstream unpaid.

## Request lifecycle

`she_jane_billing_reservations` has one row per gateway request ID and stores integer quota only:

```text
reserved -> settled
reserved -> refunded
```

- Reserve locks the wallet, rejects non-positive or oversized values, rejects insufficient quota, records
  the exact pricing snapshot, decrements the projection, and appends `reserve` atomically.
- Settlement is idempotent. It records actual quota; a positive delta appends `settle`, while unused
  reserve appends `release`. An overage that cannot be funded fails closed rather than creating debt.
- Refund is idempotent and appends `refund`; settled requests require an explicit operator/provider refund
  reference rather than reusing request failure cleanup.
- Repeating an idempotency key with different user, amount, reference, or pricing snapshot is a conflict.
- Reservation expiry/recovery is an operator job that appends a compensating entry; rows are never deleted
  to hide abandoned work.

The existing inference path is not switched to this journal until cutover and backfill reconciliation have
passed. At cutover, managed SheJane Tokens use the same model-price snapshot and existing `BillingSession`;
the journal adapter must be added at that single owner rather than in relay/provider code.

The preparation transition engine is intentionally package-private and has no controller or route. It is
covered for opening, reserve, settle, release, refund, idempotency, reconciliation, and fail-closed balance
checks, but cannot be called by the live HTTP or relay layers until the activation gate and `BillingSession`
adapter are approved together.

## Payment webhook boundary

Existing provider SDK verification remains the signature authority. After verification and before business
processing, Cloud records only provider, stable event ID, event type, SHA-256 payload digest, received time,
attempt count, and coarse processing status in `she_jane_payment_events`.

- `(provider, event_id)` is unique. A repeated event with the same digest is an auditable replay and may
  resume incomplete processing; a different digest is rejected and alerted.
- `processed` can only be written through a caller-owned transaction helper after the corresponding order
  and balance journal mutation succeeds; there is no transaction-owning shortcut that can mark it alone.
- A crash after credit but before the audit status update is safe: replay reaches the idempotent order and
  journal keys, then marks the event processed without a second credit.
- Logs never contain webhook bodies, signatures, checkout secrets, or customer payment details.
- Provider callbacks remain unavailable when compliance or provider configuration is incomplete.

No provider-neutral webhook signature algorithm is invented. Provider selection and its official webhook
contract are required before wiring these records into a live callback.

## Limits, administration, and alerts

- Negative balances and implicit credit are forbidden. Conditional updates and row locks fail closed when
  balance is insufficient.
- Operator-approved per-user balance, daily credit/spend, request reserve, and rate limits are mandatory
  activation inputs; unset limits disable paid operation.
- Existing root payment settings, top-up/order views, quota logs, and user administration are reused.
  P3 adds reconciliation/read-only journal and webhook-event views only when a provider is selected; it does
  not create a second general billing dashboard.
- Alerts cover reconciliation mismatch, repeated signature failure, digest conflict, stuck reservation,
  failed refund, provider/ledger amount mismatch, saturation, and upstream spend ceiling.

## Local verification before activation

- SQLite tests prove transactionality, idempotency, conflict rejection, insufficient-quota behavior,
  settlement/refund compensation, append-only guards, and reconciliation mismatch detection.
- Migration and GORM queries must remain valid on SQLite, MySQL 5.7.8+, and PostgreSQL 9.6+.
- MySQL trigger creation must use a direct database connection because MySQL rejects `CREATE TRIGGER`
  through the prepared-statement protocol. Verify all SheJane tables, indexes, trigger behavior, and
  repeat migration against disposable empty databases with `TEST_SHEJANE_MYSQL_DSN='<mysql-dsn>'
  TEST_SHEJANE_POSTGRES_DSN='<postgres-dsn>' go test ./model -run '^TestSheJaneMigrationsConfiguredDatabases$'`.
- Provider tests must use official signed fixtures or the provider SDK test helper once a provider is chosen.
- PostgreSQL/Redis staging proves concurrent reserve/settle/replay, cache convergence, restore/reconciliation,
  and alert delivery before any real checkout is exposed.

## External decisions that block activation, not local preparation

- commercial/project authorization and upstream resale authorization;
- jurisdiction, terms/privacy, tax/invoice, refund, identity, retention, and support decisions;
- payment provider/account, official SDK contract, currencies, minor-unit rules, webhook endpoint and keys;
- pricing version, initial/opening balances, fraud policy, all monetary/rate limits and alert destinations;
- PostgreSQL/Redis, secrets, deployment window and rollback authority.
