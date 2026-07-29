package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const paidSnapshotHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestSheJaneMigrationsConfiguredDatabases(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		dbType    common.DatabaseType
		dialector func(string) gorm.Dialector
	}{
		{name: "mysql", env: "TEST_SHEJANE_MYSQL_DSN", dbType: common.DatabaseTypeMySQL, dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{name: "postgres", env: "TEST_SHEJANE_POSTGRES_DSN", dbType: common.DatabaseTypePostgreSQL, dialector: func(dsn string) gorm.Dialector {
			return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.env))
			if dsn == "" {
				t.Skip(test.env + " is not configured")
			}
			db, err := gorm.Open(test.dialector(dsn), newGormConfig(true))
			require.NoError(t, err)
			models := []any{
				&User{}, &UserSession{}, &Token{}, &AuthFlow{}, &SheJaneDevice{},
				&SheJaneTelemetryToken{}, &SheJaneBalanceEntry{}, &SheJaneBillingReservation{},
				&SheJanePaymentEvent{},
			}
			for _, model := range models {
				require.False(t, db.Migrator().HasTable(model), "configured %s database must be empty", test.name)
			}

			previousDB := DB
			previousType := common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(test.dbType)
			initCol()
			t.Cleanup(func() {
				for index := len(models) - 1; index >= 0; index-- {
					_ = db.Migrator().DropTable(models[index])
				}
				if test.dbType == common.DatabaseTypePostgreSQL {
					_ = db.Exec("DROP FUNCTION IF EXISTS she_jane_balance_entries_immutable() CASCADE").Error
					_ = db.Exec("DROP FUNCTION IF EXISTS she_jane_paid_quota_version_guard_fn() CASCADE").Error
				}
				sqlDB, sqlErr := db.DB()
				if sqlErr == nil {
					_ = sqlDB.Close()
				}
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				initCol()
			})

			require.NoError(t, db.AutoMigrate(models...))
			require.NoError(t, ensureSheJanePaidDatabaseGuards())
			require.NoError(t, db.AutoMigrate(models...))
			require.NoError(t, ensureSheJanePaidDatabaseGuards())
			for _, index := range []struct {
				model any
				name  string
			}{
				{model: &SheJaneDevice{}, name: "idx_she_jane_devices_token_id"},
				{model: &SheJaneTelemetryToken{}, name: "idx_she_jane_telemetry_active_device"},
				{model: &SheJanePaymentEvent{}, name: "idx_she_jane_payment_provider_event"},
			} {
				assert.True(t, db.Migrator().HasIndex(index.model, index.name))
			}

			user := User{
				Username: "shejane-migration-" + test.name, Password: "unused-password",
				Status: common.UserStatusEnabled, Group: "default", Quota: 100,
				QuotaVersion: 1, SheJanePaidManaged: true,
			}
			require.NoError(t, db.Create(&user).Error)
			require.NoError(t, db.Exec(`INSERT INTO she_jane_balance_entries
				(user_id, delta, balance_after, kind, idempotency_key, reference_type, reference_id, pricing_version, pricing_snapshot_hash, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				user.Id, 100, 100, SheJaneBalanceOpening, "migration-opening-"+test.name,
				"wallet", "migration-wallet-"+test.name, "", "", 1,
			).Error)
			assert.Error(t, db.Exec("UPDATE she_jane_balance_entries SET delta = ? WHERE user_id = ?", 99, user.Id).Error)
			assert.Error(t, db.Exec("DELETE FROM she_jane_balance_entries WHERE user_id = ?", user.Id).Error)
			assert.Error(t, db.Exec("UPDATE users SET quota = ? WHERE id = ?", 90, user.Id).Error)
			require.NoError(t, db.Exec("UPDATE users SET quota = ?, quota_version = ? WHERE id = ?", 90, 2, user.Id).Error)
		})
	}
}

func setupSheJanePaidOperationsTest(t *testing.T, quota int) User {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(
		&SheJaneBalanceEntry{},
		&SheJaneBillingReservation{},
		&SheJanePaymentEvent{},
	))
	require.NoError(t, clearSheJaneBalanceEntriesForTest())
	for _, table := range []string{
		"she_jane_payment_events",
		"she_jane_billing_reservations",
	} {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	user := User{
		Username: "paid-" + strings.ReplaceAll(t.Name(), "/", "-"),
		Password: "unused-password", Status: common.UserStatusEnabled, Role: common.RoleCommonUser,
		Group: "default", Quota: quota, AffCode: "aff-" + strings.ReplaceAll(t.Name(), "/", "-"),
	}
	require.NoError(t, DB.Create(&user).Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM she_jane_payment_events")
		DB.Exec("DELETE FROM she_jane_billing_reservations")
		_ = clearSheJaneBalanceEntriesForTest()
		DB.Unscoped().Delete(&user)
	})
	return user
}

func clearSheJaneBalanceEntriesForTest() error {
	for _, statement := range []string{
		"DROP TRIGGER IF EXISTS she_jane_balance_entries_no_update",
		"DROP TRIGGER IF EXISTS she_jane_balance_entries_no_delete",
		"DELETE FROM she_jane_balance_entries",
	} {
		if err := DB.Exec(statement).Error; err != nil {
			return err
		}
	}
	return ensureSheJanePaidDatabaseGuards()
}

func TestSheJanePaidWalletReserveSettleAndRefundAreAtomicAndIdempotent(t *testing.T) {
	user := setupSheJanePaidOperationsTest(t, 1_000)
	opening, err := openSheJanePaidWallet(sheJanePaidOpeningInput{
		UserId: user.Id, IdempotencyKey: "opening:test-wallet", ReferenceId: "wallet:test", CreatedAt: 100,
	})
	require.NoError(t, err)
	assert.Equal(t, 1_000, opening.Delta)
	assert.Equal(t, 1_000, opening.BalanceAfter)

	reserve := sheJanePaidReserveInput{
		UserId: user.Id, RequestId: "request:test-1", IdempotencyKey: "reserve:test-1",
		Quota: 300, PricingVersion: "price-v1", PricingSnapshotHash: paidSnapshotHash, CreatedAt: 101,
	}
	reservation, err := reserveSheJanePaidQuota(reserve)
	require.NoError(t, err)
	assert.Equal(t, SheJaneBillingReserved, reservation.Status)
	assert.Equal(t, 700, paidTestUserQuota(t, user.Id))

	replayed, err := reserveSheJanePaidQuota(reserve)
	require.NoError(t, err)
	assert.Equal(t, reservation.Id, replayed.Id)
	conflict := reserve
	conflict.Quota = 301
	_, err = reserveSheJanePaidQuota(conflict)
	assert.ErrorIs(t, err, ErrSheJanePaidConflict)

	settled, err := settleSheJanePaidQuota(sheJanePaidSettleInput{
		RequestId: reserve.RequestId, IdempotencyKey: "settle:test-1", ActualQuota: 200, CreatedAt: 102,
	})
	require.NoError(t, err)
	assert.Equal(t, SheJaneBillingSettled, settled.Status)
	assert.Equal(t, 200, settled.ActualQuota)
	assert.Equal(t, 800, paidTestUserQuota(t, user.Id))
	_, err = settleSheJanePaidQuota(sheJanePaidSettleInput{
		RequestId: reserve.RequestId, IdempotencyKey: "settle:test-1", ActualQuota: 200, CreatedAt: 103,
	})
	require.NoError(t, err)
	_, err = settleSheJanePaidQuota(sheJanePaidSettleInput{
		RequestId: reserve.RequestId, IdempotencyKey: "settle:test-1", ActualQuota: 201, CreatedAt: 103,
	})
	assert.ErrorIs(t, err, ErrSheJanePaidConflict)
	_, err = refundSheJanePaidQuota(sheJanePaidRefundInput{
		RequestId: reserve.RequestId, IdempotencyKey: "opening:test-wallet",
		ReferenceId: "provider-refund:conflicting-key", CreatedAt: 103,
	})
	assert.ErrorIs(t, err, ErrSheJanePaidConflict)

	refunded, err := refundSheJanePaidQuota(sheJanePaidRefundInput{
		RequestId: reserve.RequestId, IdempotencyKey: "refund:test-1",
		ReferenceId: "provider-refund:test-1", CreatedAt: 104,
	})
	require.NoError(t, err)
	assert.Equal(t, SheJaneBillingRefunded, refunded.Status)
	assert.Equal(t, 1_000, paidTestUserQuota(t, user.Id))
	_, err = refundSheJanePaidQuota(sheJanePaidRefundInput{
		RequestId: reserve.RequestId, IdempotencyKey: "refund:test-1",
		ReferenceId: "provider-refund:test-1", CreatedAt: 105,
	})
	require.NoError(t, err)

	result, err := reconcileSheJanePaidWallet(user.Id)
	require.NoError(t, err)
	assert.True(t, result.Matches)
	assert.Equal(t, 1_000, result.LedgerBalance)
	assert.Equal(t, 1_000, result.UserQuota)

	var entries []SheJaneBalanceEntry
	require.NoError(t, DB.Where("user_id = ?", user.Id).Order("id ASC").Find(&entries).Error)
	require.Len(t, entries, 4)
	assert.Equal(t, []string{
		SheJaneBalanceOpening, SheJaneBalanceReserve, SheJaneBalanceRelease, SheJaneBalanceRefund,
	}, []string{entries[0].Kind, entries[1].Kind, entries[2].Kind, entries[3].Kind})
	for _, entry := range entries[1:] {
		assert.Equal(t, reserve.PricingVersion, entry.PricingVersion)
		assert.Equal(t, reserve.PricingSnapshotHash, entry.PricingSnapshotHash)
	}
}

func TestSheJanePaidWalletCannotOpenWhileLegacyBatchBillingIsEnabled(t *testing.T) {
	user := setupSheJanePaidOperationsTest(t, 100)
	oldBatchUpdate := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatchUpdate })

	_, err := openSheJanePaidWallet(sheJanePaidOpeningInput{
		UserId: user.Id, IdempotencyKey: "opening:batch-enabled", ReferenceId: "wallet:batch-enabled", CreatedAt: 100,
	})
	assert.ErrorIs(t, err, ErrSheJanePaidTransition)
}

func TestSheJanePaidReserveRollsBackQuotaWhenReservationInsertFails(t *testing.T) {
	user := setupSheJanePaidOperationsTest(t, 100)
	_, err := openSheJanePaidWallet(sheJanePaidOpeningInput{
		UserId: user.Id, IdempotencyKey: "opening:rollback", ReferenceId: "wallet:rollback", CreatedAt: 100,
	})
	require.NoError(t, err)
	require.NoError(t, DB.Exec(`CREATE TRIGGER fail_she_jane_reservation_insert
		BEFORE INSERT ON she_jane_billing_reservations
		WHEN NEW.request_id = 'request:rollback'
		BEGIN SELECT RAISE(ABORT, 'forced reservation failure'); END`).Error)
	t.Cleanup(func() { DB.Exec("DROP TRIGGER IF EXISTS fail_she_jane_reservation_insert") })

	_, err = reserveSheJanePaidQuota(sheJanePaidReserveInput{
		UserId: user.Id, RequestId: "request:rollback", IdempotencyKey: "reserve:rollback",
		Quota: 40, PricingVersion: "price-v1", PricingSnapshotHash: paidSnapshotHash, CreatedAt: 101,
	})
	require.Error(t, err)
	assert.Equal(t, 100, paidTestUserQuota(t, user.Id))
	var reservations, entries int64
	require.NoError(t, DB.Model(&SheJaneBillingReservation{}).Where("request_id = ?", "request:rollback").Count(&reservations).Error)
	require.NoError(t, DB.Model(&SheJaneBalanceEntry{}).Where("idempotency_key = ?", "reserve:rollback").Count(&entries).Error)
	assert.Zero(t, reservations)
	assert.Zero(t, entries)
}

func TestSheJanePaidWalletFailsClosedOnInsufficientQuotaAndMismatch(t *testing.T) {
	user := setupSheJanePaidOperationsTest(t, 100)
	_, err := openSheJanePaidWallet(sheJanePaidOpeningInput{
		UserId: user.Id, IdempotencyKey: "opening:fail-closed", ReferenceId: "wallet:fail-closed", CreatedAt: 100,
	})
	require.NoError(t, err)

	input := sheJanePaidReserveInput{
		UserId: user.Id, RequestId: "request:too-large", IdempotencyKey: "reserve:too-large",
		Quota: 101, PricingVersion: "price-v1", PricingSnapshotHash: paidSnapshotHash, CreatedAt: 101,
	}
	_, err = reserveSheJanePaidQuota(input)
	assert.ErrorIs(t, err, ErrSheJanePaidInsufficientQuota)
	assert.Equal(t, 100, paidTestUserQuota(t, user.Id))

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"quota": 99, "quota_version": gorm.Expr("quota_version + ?", 1),
	}).Error)
	result, err := reconcileSheJanePaidWallet(user.Id)
	require.NoError(t, err)
	assert.False(t, result.Matches)
	input.Quota = 1
	input.RequestId = "request:mismatch"
	input.IdempotencyKey = "reserve:mismatch"
	_, err = reserveSheJanePaidQuota(input)
	assert.ErrorIs(t, err, ErrSheJanePaidReconciliation)
}

func TestSheJanePaidSettlementRejectsUnfundedOverageAndCanRefundReservation(t *testing.T) {
	user := setupSheJanePaidOperationsTest(t, 100)
	_, err := openSheJanePaidWallet(sheJanePaidOpeningInput{
		UserId: user.Id, IdempotencyKey: "opening:overage", ReferenceId: "wallet:overage", CreatedAt: 100,
	})
	require.NoError(t, err)
	_, err = reserveSheJanePaidQuota(sheJanePaidReserveInput{
		UserId: user.Id, RequestId: "request:overage", IdempotencyKey: "reserve:overage",
		Quota: 80, PricingVersion: "price-v1", PricingSnapshotHash: paidSnapshotHash, CreatedAt: 101,
	})
	require.NoError(t, err)

	_, err = settleSheJanePaidQuota(sheJanePaidSettleInput{
		RequestId: "request:overage", IdempotencyKey: "settle:overage", ActualQuota: 101, CreatedAt: 102,
	})
	assert.ErrorIs(t, err, ErrSheJanePaidInsufficientQuota)
	assert.Equal(t, 20, paidTestUserQuota(t, user.Id))
	var reservation SheJaneBillingReservation
	require.NoError(t, DB.Where("request_id = ?", "request:overage").First(&reservation).Error)
	assert.Equal(t, SheJaneBillingReserved, reservation.Status)

	_, err = refundSheJanePaidQuota(sheJanePaidRefundInput{
		RequestId: "request:overage", IdempotencyKey: "refund:overage",
		ReferenceId: "operator-refund:overage", CreatedAt: 103,
	})
	require.NoError(t, err)
	assert.Equal(t, 100, paidTestUserQuota(t, user.Id))
}

func TestSheJanePaidSettlementRecordsFundedOverage(t *testing.T) {
	user := setupSheJanePaidOperationsTest(t, 1_000)
	_, err := openSheJanePaidWallet(sheJanePaidOpeningInput{
		UserId: user.Id, IdempotencyKey: "opening:funded-overage", ReferenceId: "wallet:funded-overage", CreatedAt: 100,
	})
	require.NoError(t, err)
	_, err = reserveSheJanePaidQuota(sheJanePaidReserveInput{
		UserId: user.Id, RequestId: "request:funded-overage", IdempotencyKey: "reserve:funded-overage",
		Quota: 200, PricingVersion: "price-v1", PricingSnapshotHash: paidSnapshotHash, CreatedAt: 101,
	})
	require.NoError(t, err)
	_, err = settleSheJanePaidQuota(sheJanePaidSettleInput{
		RequestId: "request:funded-overage", IdempotencyKey: "settle:funded-overage", ActualQuota: 250, CreatedAt: 102,
	})
	require.NoError(t, err)

	var entry SheJaneBalanceEntry
	require.NoError(t, DB.Where("idempotency_key = ?", "settle:funded-overage").First(&entry).Error)
	assert.Equal(t, SheJaneBalanceSettle, entry.Kind)
	assert.Equal(t, -50, entry.Delta)
	assert.Equal(t, 750, entry.BalanceAfter)
	assert.Equal(t, "price-v1", entry.PricingVersion)
	assert.Equal(t, paidSnapshotHash, entry.PricingSnapshotHash)
}

func TestSheJanePaidAdjustmentRepairsProjectionWithoutEditingHistory(t *testing.T) {
	user := setupSheJanePaidOperationsTest(t, 100)
	_, err := openSheJanePaidWallet(sheJanePaidOpeningInput{
		UserId: user.Id, IdempotencyKey: "opening:adjustment", ReferenceId: "wallet:adjustment", CreatedAt: 100,
	})
	require.NoError(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"quota": 99, "quota_version": gorm.Expr("quota_version + ?", 1),
	}).Error)

	entry, err := adjustSheJanePaidQuota(sheJanePaidAdjustmentInput{
		UserId: user.Id, IdempotencyKey: "adjustment:projection", ReferenceId: "operator-case:1",
		TargetQuota: 99, CreatedAt: 101,
	})
	require.NoError(t, err)
	assert.Equal(t, SheJaneBalanceAdjustment, entry.Kind)
	assert.Equal(t, -1, entry.Delta)
	assert.Equal(t, 99, entry.BalanceAfter)

	result, err := reconcileSheJanePaidWallet(user.Id)
	require.NoError(t, err)
	assert.True(t, result.Matches)
	assert.True(t, result.LedgerValid)
}

func TestSheJanePaidQuotaVersionRejectsDelayedStaleCacheFill(t *testing.T) {
	user := setupSheJanePaidOperationsTest(t, 100)
	useUserCacheMiniRedis(t)
	require.NoError(t, populateUserCache(user))
	stale := *user.ToBaseUser()
	_, err := openSheJanePaidWallet(sheJanePaidOpeningInput{
		UserId: user.Id, IdempotencyKey: "opening:quota-fence", ReferenceId: "wallet:quota-fence", CreatedAt: 100,
	})
	require.NoError(t, err)
	_, err = reserveSheJanePaidQuota(sheJanePaidReserveInput{
		UserId: user.Id, RequestId: "request:quota-fence", IdempotencyKey: "reserve:quota-fence",
		Quota: 40, PricingVersion: "price-v1", PricingSnapshotHash: paidSnapshotHash, CreatedAt: 101,
	})
	require.NoError(t, err)

	err = writeUserCache(&stale, true)
	assert.ErrorIs(t, err, ErrUserQuotaCachePending)
	_, err = cacheGetUserBase(user.Id)
	assert.ErrorIs(t, err, ErrUserQuotaCacheBypass)
	quota, err := GetUserQuota(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 60, quota)

	assert.Error(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", 55).Error)
	assert.Error(t, DecreaseUserQuota(user.Id, 1, true))
	oldBatchUpdate := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatchUpdate })
	assert.ErrorIs(t, DecreaseUserQuota(user.Id, 1, false), ErrSheJanePaidTransition)
	require.NoError(t, common.RDB.HSet(t.Context(), getUserCacheKey(user.Id), "Quota", 999).Err())
	quota, err = GetUserQuota(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 60, quota)
}

func TestSheJaneBalanceJournalRejectsUpdateAndDelete(t *testing.T) {
	user := setupSheJanePaidOperationsTest(t, 100)
	direct := SheJaneBalanceEntry{
		UserId: user.Id, Delta: 100, BalanceAfter: 100, Kind: SheJaneBalanceOpening,
		IdempotencyKey: "opening:direct", ReferenceType: "wallet", ReferenceId: "wallet:direct", CreatedAt: 99,
	}
	assert.ErrorIs(t, DB.Create(&direct).Error, ErrSheJaneBalanceEntryImmutable)
	entry, err := openSheJanePaidWallet(sheJanePaidOpeningInput{
		UserId: user.Id, IdempotencyKey: "opening:immutable", ReferenceId: "wallet:immutable", CreatedAt: 100,
	})
	require.NoError(t, err)

	assert.ErrorIs(t, DB.Model(entry).Update("balance_after", 0).Error, ErrSheJaneBalanceEntryImmutable)
	assert.ErrorIs(t, DB.Delete(entry).Error, ErrSheJaneBalanceEntryImmutable)
	assert.Error(t, DB.Session(&gorm.Session{SkipHooks: true}).Model(entry).UpdateColumn("balance_after", 0).Error)
	assert.Error(t, DB.Exec("DELETE FROM she_jane_balance_entries WHERE id = ?", entry.Id).Error)
}

func TestSheJanePaidDatabaseGuardsRejectWrongExistingTrigger(t *testing.T) {
	setupSheJanePaidOperationsTest(t, 0)
	require.NoError(t, DB.Exec("DROP TRIGGER IF EXISTS she_jane_paid_quota_version_guard").Error)
	require.NoError(t, DB.Exec(`CREATE TRIGGER she_jane_paid_quota_version_guard BEFORE UPDATE OF quota ON users BEGIN SELECT 1; END`).Error)
	t.Cleanup(func() {
		DB.Exec("DROP TRIGGER IF EXISTS she_jane_paid_quota_version_guard")
		_ = clearSheJaneBalanceEntriesForTest()
	})

	assert.Error(t, ensureSheJanePaidDatabaseGuards())
	require.NoError(t, DB.Exec("DROP TRIGGER IF EXISTS she_jane_paid_quota_version_guard").Error)
	require.NoError(t, ensureSheJanePaidDatabaseGuards())
	require.NoError(t, DB.Exec("DROP TRIGGER IF EXISTS she_jane_balance_entries_no_update").Error)
	require.NoError(t, DB.Exec(`CREATE TRIGGER she_jane_balance_entries_no_update BEFORE UPDATE ON she_jane_balance_entries BEGIN SELECT 1; END`).Error)
	assert.Error(t, ensureSheJanePaidDatabaseGuards())
}

func TestSheJanePaymentEventReplayIsAuditableAndDigestConflictFailsClosed(t *testing.T) {
	setupSheJanePaidOperationsTest(t, 0)
	input := sheJanePaymentEventInput{
		Provider: "provider_test", EventId: "evt:test-1", EventType: "payment.completed",
		PayloadSHA256: paidSnapshotHash, ReceivedAt: 100,
	}
	event, replay, err := recordSheJanePaymentEvent(input)
	require.NoError(t, err)
	assert.False(t, replay)
	assert.Equal(t, 1, event.AttemptCount)

	event, replay, err = recordSheJanePaymentEvent(input)
	require.NoError(t, err)
	assert.True(t, replay)
	assert.Equal(t, 2, event.AttemptCount)
	conflict := input
	conflict.PayloadSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	_, _, err = recordSheJanePaymentEvent(conflict)
	assert.ErrorIs(t, err, ErrSheJanePaidConflict)

	var processed SheJanePaymentEvent
	err = DB.Transaction(func(tx *gorm.DB) error {
		var markErr error
		processed, markErr = markSheJanePaymentEventProcessedWithTx(tx, input.Provider, input.EventId, 101)
		return markErr
	})
	require.NoError(t, err)
	assert.Equal(t, SheJanePaymentProcessed, processed.Status)
	assert.Equal(t, int64(101), processed.ProcessedAt)
}

func TestSheJanePaymentEventRejectsInvalidProcessingTimeAndAttemptOverflow(t *testing.T) {
	setupSheJanePaidOperationsTest(t, 0)
	input := sheJanePaymentEventInput{
		Provider: "provider_test", EventId: "evt:bounds", EventType: "payment.completed",
		PayloadSHA256: paidSnapshotHash, ReceivedAt: 100,
	}
	event, _, err := recordSheJanePaymentEvent(input)
	require.NoError(t, err)
	err = DB.Transaction(func(tx *gorm.DB) error {
		_, markErr := markSheJanePaymentEventProcessedWithTx(tx, input.Provider, input.EventId, 99)
		return markErr
	})
	assert.ErrorIs(t, err, ErrSheJanePaidInvalid)

	require.NoError(t, DB.Model(event).Update("attempt_count", common.MaxQuota).Error)
	_, _, err = recordSheJanePaymentEvent(input)
	assert.ErrorIs(t, err, ErrSheJanePaidInvalid)
}

func paidTestUserQuota(t *testing.T, userId int) int {
	t.Helper()
	var quota int
	require.NoError(t, DB.Model(&User{}).Where("id = ?", userId).Select("quota").Scan(&quota).Error)
	return quota
}
