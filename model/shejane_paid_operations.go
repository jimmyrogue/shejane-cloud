package model

import (
	"errors"
	"fmt"
	"math"
	"regexp"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SheJaneBalanceOpening    = "opening"
	SheJaneBalanceReserve    = "reserve"
	SheJaneBalanceSettle     = "settle"
	SheJaneBalanceRelease    = "release"
	SheJaneBalanceRefund     = "refund"
	SheJaneBalanceAdjustment = "adjustment"

	SheJaneBillingReserved = "reserved"
	SheJaneBillingSettled  = "settled"
	SheJaneBillingRefunded = "refunded"

	SheJanePaymentReceived  = "received"
	SheJanePaymentProcessed = "processed"
)

var (
	ErrSheJanePaidInvalid            = errors.New("invalid SheJane paid operation")
	ErrSheJanePaidConflict           = errors.New("SheJane paid operation conflict")
	ErrSheJanePaidInsufficientQuota  = errors.New("insufficient SheJane paid quota")
	ErrSheJanePaidReconciliation     = errors.New("SheJane paid wallet reconciliation mismatch")
	ErrSheJanePaidTransition         = errors.New("invalid SheJane paid operation transition")
	ErrSheJaneBalanceEntryImmutable  = errors.New("SheJane balance entries are immutable")
	sheJanePaidIdentifierPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
	sheJanePaidPricingVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	sheJanePaidProviderPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	sheJanePaidSHA256Pattern         = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type SheJaneBalanceEntry struct {
	Id                  int64  `json:"id" gorm:"primaryKey"`
	UserId              int    `json:"user_id" gorm:"not null;index"`
	Delta               int    `json:"delta" gorm:"type:int;not null"`
	BalanceAfter        int    `json:"balance_after" gorm:"type:int;not null"`
	Kind                string `json:"kind" gorm:"type:varchar(16);not null;index"`
	IdempotencyKey      string `json:"idempotency_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	ReferenceType       string `json:"reference_type" gorm:"type:varchar(32);not null"`
	ReferenceId         string `json:"reference_id" gorm:"type:varchar(128);not null;index"`
	PricingVersion      string `json:"pricing_version,omitempty" gorm:"type:varchar(64);not null"`
	PricingSnapshotHash string `json:"pricing_snapshot_hash,omitempty" gorm:"type:char(64);not null"`
	CreatedAt           int64  `json:"created_at" gorm:"type:bigint;not null;index"`
	allowCreate         bool   `json:"-" gorm:"-"`
}

func (SheJaneBalanceEntry) TableName() string { return "she_jane_balance_entries" }

func (entry *SheJaneBalanceEntry) BeforeCreate(*gorm.DB) error {
	if !entry.allowCreate {
		return ErrSheJaneBalanceEntryImmutable
	}
	return nil
}

func (*SheJaneBalanceEntry) BeforeUpdate(*gorm.DB) error {
	return ErrSheJaneBalanceEntryImmutable
}

func (*SheJaneBalanceEntry) BeforeDelete(*gorm.DB) error {
	return ErrSheJaneBalanceEntryImmutable
}

type SheJaneBillingReservation struct {
	Id                  int64   `json:"id" gorm:"primaryKey"`
	UserId              int     `json:"user_id" gorm:"not null;index"`
	RequestId           string  `json:"request_id" gorm:"type:varchar(128);not null;uniqueIndex"`
	ReserveKey          string  `json:"reserve_key" gorm:"type:varchar(128);not null;uniqueIndex"`
	SettleKey           *string `json:"settle_key,omitempty" gorm:"type:varchar(128);uniqueIndex"`
	RefundKey           *string `json:"refund_key,omitempty" gorm:"type:varchar(128);uniqueIndex"`
	ReservedQuota       int     `json:"reserved_quota" gorm:"type:int;not null"`
	ActualQuota         int     `json:"actual_quota" gorm:"type:int;not null"`
	Status              string  `json:"status" gorm:"type:varchar(16);not null;index"`
	PricingVersion      string  `json:"pricing_version" gorm:"type:varchar(64);not null"`
	PricingSnapshotHash string  `json:"pricing_snapshot_hash" gorm:"type:char(64);not null"`
	RefundReferenceId   string  `json:"refund_reference_id,omitempty" gorm:"type:varchar(128);not null"`
	CreatedAt           int64   `json:"created_at" gorm:"type:bigint;not null;index"`
	UpdatedAt           int64   `json:"updated_at" gorm:"type:bigint;not null;index"`
}

func (SheJaneBillingReservation) TableName() string {
	return "she_jane_billing_reservations"
}

type SheJanePaymentEvent struct {
	Id            int64  `json:"id" gorm:"primaryKey"`
	Provider      string `json:"provider" gorm:"type:varchar(32);not null;uniqueIndex:idx_she_jane_payment_provider_event"`
	EventId       string `json:"event_id" gorm:"type:varchar(128);not null;uniqueIndex:idx_she_jane_payment_provider_event"`
	EventType     string `json:"event_type" gorm:"type:varchar(128);not null"`
	PayloadSHA256 string `json:"payload_sha256" gorm:"type:char(64);not null"`
	Status        string `json:"status" gorm:"type:varchar(16);not null;index"`
	AttemptCount  int    `json:"attempt_count" gorm:"type:int;not null"`
	ReceivedAt    int64  `json:"received_at" gorm:"type:bigint;not null;index"`
	LastAttemptAt int64  `json:"last_attempt_at" gorm:"type:bigint;not null"`
	ProcessedAt   int64  `json:"processed_at" gorm:"type:bigint;not null"`
}

func (SheJanePaymentEvent) TableName() string { return "she_jane_payment_events" }

type sheJanePaidOpeningInput struct {
	UserId         int
	IdempotencyKey string
	ReferenceId    string
	CreatedAt      int64
}

type sheJanePaidReserveInput struct {
	UserId              int
	RequestId           string
	IdempotencyKey      string
	Quota               int
	PricingVersion      string
	PricingSnapshotHash string
	CreatedAt           int64
}

type sheJanePaidSettleInput struct {
	RequestId      string
	IdempotencyKey string
	ActualQuota    int
	CreatedAt      int64
}

type sheJanePaidRefundInput struct {
	RequestId      string
	IdempotencyKey string
	ReferenceId    string
	CreatedAt      int64
}

type sheJanePaidAdjustmentInput struct {
	UserId         int
	IdempotencyKey string
	ReferenceId    string
	TargetQuota    int
	CreatedAt      int64
}

type SheJanePaidReconciliation struct {
	Matches       bool
	LedgerValid   bool
	UserQuota     int
	LedgerBalance int
	EntryCount    int
}

type sheJanePaymentEventInput struct {
	Provider      string
	EventId       string
	EventType     string
	PayloadSHA256 string
	ReceivedAt    int64
}

func openSheJanePaidWallet(input sheJanePaidOpeningInput) (*SheJaneBalanceEntry, error) {
	if input.UserId <= 0 || !validSheJanePaidIdentifier(input.IdempotencyKey) || !validSheJanePaidIdentifier(input.ReferenceId) || input.CreatedAt <= 0 {
		return nil, ErrSheJanePaidInvalid
	}
	if common.BatchUpdateEnabled {
		return nil, ErrSheJanePaidTransition
	}
	var entry SheJaneBalanceEntry
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Where("id = ?", input.UserId).First(&user).Error; err != nil {
			return err
		}
		if user.Quota < 0 || user.Quota > common.MaxQuota {
			return ErrSheJanePaidInvalid
		}
		var existing SheJaneBalanceEntry
		err := tx.Where("idempotency_key = ?", input.IdempotencyKey).First(&existing).Error
		if err == nil {
			if existing.UserId != input.UserId || existing.Kind != SheJaneBalanceOpening || existing.ReferenceId != input.ReferenceId {
				return ErrSheJanePaidConflict
			}
			if !user.SheJanePaidManaged {
				if err := enableSheJanePaidWalletWithTx(tx, &user); err != nil {
					return err
				}
			}
			entry = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err := tx.Model(&SheJaneBalanceEntry{}).Where("user_id = ?", input.UserId).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrSheJanePaidConflict
		}
		if err := enableSheJanePaidWalletWithTx(tx, &user); err != nil {
			return err
		}
		entry = SheJaneBalanceEntry{
			UserId: input.UserId, Delta: user.Quota, BalanceAfter: user.Quota,
			Kind: SheJaneBalanceOpening, IdempotencyKey: input.IdempotencyKey,
			ReferenceType: "wallet", ReferenceId: input.ReferenceId, CreatedAt: input.CreatedAt,
			allowCreate: true,
		}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected != 1 {
			return ErrSheJanePaidConflict
		}
		return nil
	})
	if err == nil {
		err = PublishUserQuotaCache(input.UserId)
	}
	return &entry, err
}

func reserveSheJanePaidQuota(input sheJanePaidReserveInput) (*SheJaneBillingReservation, error) {
	if input.UserId <= 0 || !validSheJanePaidIdentifier(input.RequestId) || !validSheJanePaidIdentifier(input.IdempotencyKey) || input.Quota <= 0 || input.Quota > common.MaxQuota || !sheJanePaidPricingVersionPattern.MatchString(input.PricingVersion) || !sheJanePaidSHA256Pattern.MatchString(input.PricingSnapshotHash) || input.CreatedAt <= 0 {
		return nil, ErrSheJanePaidInvalid
	}
	var reservation SheJaneBillingReservation
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Where("id = ?", input.UserId).First(&user).Error; err != nil {
			return err
		}
		var existing SheJaneBillingReservation
		err := tx.Where("request_id = ? OR reserve_key = ?", input.RequestId, input.IdempotencyKey).First(&existing).Error
		if err == nil {
			if existing.UserId != input.UserId || existing.RequestId != input.RequestId || existing.ReserveKey != input.IdempotencyKey || existing.ReservedQuota != input.Quota || existing.PricingVersion != input.PricingVersion || existing.PricingSnapshotHash != input.PricingSnapshotHash {
				return ErrSheJanePaidConflict
			}
			reservation = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		reconciliation, err := reconcileSheJanePaidWalletWithTx(tx, &user)
		if err != nil {
			return err
		}
		if !reconciliation.Matches {
			return ErrSheJanePaidReconciliation
		}
		if user.Quota < input.Quota {
			return ErrSheJanePaidInsufficientQuota
		}
		balanceAfter := user.Quota - input.Quota
		if err := updateSheJanePaidQuotaWithTx(tx, &user, balanceAfter); err != nil {
			return err
		}
		reservation = SheJaneBillingReservation{
			UserId: input.UserId, RequestId: input.RequestId, ReserveKey: input.IdempotencyKey,
			ReservedQuota: input.Quota, Status: SheJaneBillingReserved,
			PricingVersion: input.PricingVersion, PricingSnapshotHash: input.PricingSnapshotHash,
			CreatedAt: input.CreatedAt, UpdatedAt: input.CreatedAt,
		}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&reservation)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected != 1 {
			return ErrSheJanePaidConflict
		}
		return createSheJaneBalanceEntry(tx, SheJaneBalanceEntry{
			UserId: input.UserId, Delta: -input.Quota, BalanceAfter: balanceAfter,
			Kind: SheJaneBalanceReserve, IdempotencyKey: input.IdempotencyKey,
			ReferenceType: "request", ReferenceId: input.RequestId,
			PricingVersion: input.PricingVersion, PricingSnapshotHash: input.PricingSnapshotHash,
			CreatedAt: input.CreatedAt,
		})
	})
	if errors.Is(err, ErrSheJanePaidConflict) {
		var existing SheJaneBillingReservation
		queryErr := DB.Where("request_id = ? OR reserve_key = ?", input.RequestId, input.IdempotencyKey).First(&existing).Error
		if queryErr == nil && existing.UserId == input.UserId && existing.RequestId == input.RequestId && existing.ReserveKey == input.IdempotencyKey && existing.ReservedQuota == input.Quota && existing.PricingVersion == input.PricingVersion && existing.PricingSnapshotHash == input.PricingSnapshotHash {
			reservation = existing
			err = nil
		}
	}
	if err == nil {
		err = PublishUserQuotaCache(input.UserId)
	}
	return &reservation, err
}

func settleSheJanePaidQuota(input sheJanePaidSettleInput) (*SheJaneBillingReservation, error) {
	if !validSheJanePaidIdentifier(input.RequestId) || !validSheJanePaidIdentifier(input.IdempotencyKey) || input.ActualQuota < 0 || input.ActualQuota > common.MaxQuota || input.CreatedAt <= 0 {
		return nil, ErrSheJanePaidInvalid
	}
	var reservation SheJaneBillingReservation
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("request_id = ?", input.RequestId).First(&reservation).Error; err != nil {
			return err
		}
		var user User
		if err := lockForUpdate(tx).Where("id = ?", reservation.UserId).First(&user).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("id = ?", reservation.Id).First(&reservation).Error; err != nil {
			return err
		}
		if reservation.Status == SheJaneBillingSettled {
			if reservation.SettleKey != nil && *reservation.SettleKey == input.IdempotencyKey && reservation.ActualQuota == input.ActualQuota {
				return nil
			}
			return ErrSheJanePaidConflict
		}
		if reservation.Status != SheJaneBillingReserved {
			return ErrSheJanePaidTransition
		}
		if err := ensureSheJaneReservationKeyAvailable(tx, "settle_key", input.IdempotencyKey, reservation.Id); err != nil {
			return err
		}
		reconciliation, err := reconcileSheJanePaidWalletWithTx(tx, &user)
		if err != nil {
			return err
		}
		if !reconciliation.Matches {
			return ErrSheJanePaidReconciliation
		}
		balanceDelta := reservation.ReservedQuota - input.ActualQuota
		balanceAfter, err := checkedSheJanePaidBalance(user.Quota, balanceDelta)
		if err != nil {
			return err
		}
		if err := updateSheJanePaidQuotaWithTx(tx, &user, balanceAfter); err != nil {
			return err
		}
		kind := SheJaneBalanceSettle
		if balanceDelta > 0 {
			kind = SheJaneBalanceRelease
		}
		if err := createSheJaneBalanceEntry(tx, SheJaneBalanceEntry{
			UserId: reservation.UserId, Delta: balanceDelta, BalanceAfter: balanceAfter,
			Kind: kind, IdempotencyKey: input.IdempotencyKey,
			ReferenceType: "request", ReferenceId: reservation.RequestId,
			PricingVersion: reservation.PricingVersion, PricingSnapshotHash: reservation.PricingSnapshotHash,
			CreatedAt: input.CreatedAt,
		}); err != nil {
			return err
		}
		reservation.Status = SheJaneBillingSettled
		reservation.ActualQuota = input.ActualQuota
		reservation.SettleKey = &input.IdempotencyKey
		reservation.UpdatedAt = input.CreatedAt
		return tx.Model(&reservation).Updates(map[string]any{
			"status": reservation.Status, "actual_quota": reservation.ActualQuota,
			"settle_key": input.IdempotencyKey, "updated_at": input.CreatedAt,
		}).Error
	})
	if err == nil {
		err = PublishUserQuotaCache(reservation.UserId)
	}
	return &reservation, err
}

func refundSheJanePaidQuota(input sheJanePaidRefundInput) (*SheJaneBillingReservation, error) {
	if !validSheJanePaidIdentifier(input.RequestId) || !validSheJanePaidIdentifier(input.IdempotencyKey) || !validSheJanePaidIdentifier(input.ReferenceId) || input.CreatedAt <= 0 {
		return nil, ErrSheJanePaidInvalid
	}
	var reservation SheJaneBillingReservation
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("request_id = ?", input.RequestId).First(&reservation).Error; err != nil {
			return err
		}
		var user User
		if err := lockForUpdate(tx).Where("id = ?", reservation.UserId).First(&user).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("id = ?", reservation.Id).First(&reservation).Error; err != nil {
			return err
		}
		if reservation.Status == SheJaneBillingRefunded {
			if reservation.RefundKey != nil && *reservation.RefundKey == input.IdempotencyKey && reservation.RefundReferenceId == input.ReferenceId {
				return nil
			}
			return ErrSheJanePaidConflict
		}
		if reservation.Status != SheJaneBillingReserved && reservation.Status != SheJaneBillingSettled {
			return ErrSheJanePaidTransition
		}
		if err := ensureSheJaneReservationKeyAvailable(tx, "refund_key", input.IdempotencyKey, reservation.Id); err != nil {
			return err
		}
		reconciliation, err := reconcileSheJanePaidWalletWithTx(tx, &user)
		if err != nil {
			return err
		}
		if !reconciliation.Matches {
			return ErrSheJanePaidReconciliation
		}
		refundQuota := reservation.ReservedQuota
		if reservation.Status == SheJaneBillingSettled {
			refundQuota = reservation.ActualQuota
		}
		balanceAfter, err := checkedSheJanePaidBalance(user.Quota, refundQuota)
		if err != nil {
			return err
		}
		if err := updateSheJanePaidQuotaWithTx(tx, &user, balanceAfter); err != nil {
			return err
		}
		if err := createSheJaneBalanceEntry(tx, SheJaneBalanceEntry{
			UserId: reservation.UserId, Delta: refundQuota, BalanceAfter: balanceAfter,
			Kind: SheJaneBalanceRefund, IdempotencyKey: input.IdempotencyKey,
			ReferenceType: "refund", ReferenceId: input.ReferenceId,
			PricingVersion: reservation.PricingVersion, PricingSnapshotHash: reservation.PricingSnapshotHash,
			CreatedAt: input.CreatedAt,
		}); err != nil {
			return err
		}
		reservation.Status = SheJaneBillingRefunded
		reservation.RefundKey = &input.IdempotencyKey
		reservation.RefundReferenceId = input.ReferenceId
		reservation.UpdatedAt = input.CreatedAt
		return tx.Model(&reservation).Updates(map[string]any{
			"status": reservation.Status, "refund_key": input.IdempotencyKey,
			"refund_reference_id": input.ReferenceId, "updated_at": input.CreatedAt,
		}).Error
	})
	if err == nil {
		err = PublishUserQuotaCache(reservation.UserId)
	}
	return &reservation, err
}

func adjustSheJanePaidQuota(input sheJanePaidAdjustmentInput) (*SheJaneBalanceEntry, error) {
	if input.UserId <= 0 || !validSheJanePaidIdentifier(input.IdempotencyKey) || !validSheJanePaidIdentifier(input.ReferenceId) || input.TargetQuota < 0 || input.TargetQuota > common.MaxQuota || input.CreatedAt <= 0 {
		return nil, ErrSheJanePaidInvalid
	}
	var entry SheJaneBalanceEntry
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Where("id = ?", input.UserId).First(&user).Error; err != nil {
			return err
		}
		var existing SheJaneBalanceEntry
		err := tx.Where("idempotency_key = ?", input.IdempotencyKey).First(&existing).Error
		if err == nil {
			if existing.UserId != input.UserId || existing.Kind != SheJaneBalanceAdjustment || existing.ReferenceId != input.ReferenceId || existing.BalanceAfter != input.TargetQuota {
				return ErrSheJanePaidConflict
			}
			entry = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		reconciliation, err := reconcileSheJanePaidWalletWithTx(tx, &user)
		if err != nil {
			return err
		}
		if !reconciliation.LedgerValid {
			return ErrSheJanePaidReconciliation
		}
		delta := int64(input.TargetQuota) - int64(reconciliation.LedgerBalance)
		if delta < -int64(common.MaxQuota) || delta > int64(common.MaxQuota) {
			return ErrSheJanePaidInvalid
		}
		if err := updateSheJanePaidQuotaWithTx(tx, &user, input.TargetQuota); err != nil {
			return err
		}
		entry = SheJaneBalanceEntry{
			UserId: input.UserId, Delta: int(delta), BalanceAfter: input.TargetQuota,
			Kind: SheJaneBalanceAdjustment, IdempotencyKey: input.IdempotencyKey,
			ReferenceType: "operator", ReferenceId: input.ReferenceId, CreatedAt: input.CreatedAt,
		}
		return createSheJaneBalanceEntry(tx, entry)
	})
	if err == nil {
		err = PublishUserQuotaCache(input.UserId)
	}
	return &entry, err
}

func reconcileSheJanePaidWallet(userId int) (*SheJanePaidReconciliation, error) {
	if userId <= 0 {
		return nil, ErrSheJanePaidInvalid
	}
	var reconciliation *SheJanePaidReconciliation
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		var err error
		reconciliation, err = reconcileSheJanePaidWalletWithTx(tx, &user)
		return err
	})
	return reconciliation, err
}

func reconcileSheJanePaidWalletWithTx(tx *gorm.DB, user *User) (*SheJanePaidReconciliation, error) {
	var entries []SheJaneBalanceEntry
	if err := tx.Where("user_id = ?", user.Id).Order("id ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	result := &SheJanePaidReconciliation{UserQuota: user.Quota, EntryCount: len(entries)}
	if len(entries) == 0 {
		return result, nil
	}
	if entries[0].Kind != SheJaneBalanceOpening {
		common.SysError(fmt.Sprintf("SheJane paid wallet reconciliation alert: user_id=%d invalid opening entry", user.Id))
		return result, nil
	}
	running := int64(0)
	for _, entry := range entries {
		running += int64(entry.Delta)
		if running < 0 || running > int64(common.MaxQuota) || int(running) != entry.BalanceAfter {
			common.SysError(fmt.Sprintf("SheJane paid wallet reconciliation alert: user_id=%d invalid journal chain", user.Id))
			return result, nil
		}
	}
	result.LedgerBalance = int(running)
	result.LedgerValid = true
	result.Matches = result.LedgerBalance == user.Quota
	if !result.Matches {
		common.SysError(fmt.Sprintf("SheJane paid wallet reconciliation alert: user_id=%d projection mismatch", user.Id))
	}
	return result, nil
}

func recordSheJanePaymentEvent(input sheJanePaymentEventInput) (*SheJanePaymentEvent, bool, error) {
	if !sheJanePaidProviderPattern.MatchString(input.Provider) || !validSheJanePaidIdentifier(input.EventId) || !validSheJanePaidIdentifier(input.EventType) || !sheJanePaidSHA256Pattern.MatchString(input.PayloadSHA256) || input.ReceivedAt <= 0 {
		return nil, false, ErrSheJanePaidInvalid
	}
	event := SheJanePaymentEvent{
		Provider: input.Provider, EventId: input.EventId, EventType: input.EventType,
		PayloadSHA256: input.PayloadSHA256, Status: SheJanePaymentReceived,
		AttemptCount: 1, ReceivedAt: input.ReceivedAt, LastAttemptAt: input.ReceivedAt,
	}
	created := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider"}, {Name: "event_id"}},
		DoNothing: true,
	}).Create(&event)
	if created.Error != nil {
		return nil, false, created.Error
	}
	if created.RowsAffected == 1 {
		return &event, false, nil
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("provider = ? AND event_id = ?", input.Provider, input.EventId).First(&event).Error; err != nil {
			return err
		}
		if event.EventType != input.EventType || event.PayloadSHA256 != input.PayloadSHA256 {
			common.SysError(fmt.Sprintf("SheJane payment webhook digest conflict alert: provider=%s", input.Provider))
			return ErrSheJanePaidConflict
		}
		if input.ReceivedAt < event.ReceivedAt || input.ReceivedAt < event.LastAttemptAt || event.AttemptCount >= common.MaxQuota {
			return ErrSheJanePaidInvalid
		}
		event.AttemptCount++
		event.LastAttemptAt = input.ReceivedAt
		return tx.Model(&event).Updates(map[string]any{
			"attempt_count":   event.AttemptCount,
			"last_attempt_at": event.LastAttemptAt,
		}).Error
	})
	return &event, true, err
}

func markSheJanePaymentEventProcessedWithTx(tx *gorm.DB, provider, eventId string, processedAt int64) (SheJanePaymentEvent, error) {
	var event SheJanePaymentEvent
	if tx == nil || !sheJanePaidProviderPattern.MatchString(provider) || !validSheJanePaidIdentifier(eventId) || processedAt <= 0 {
		return event, ErrSheJanePaidInvalid
	}
	if err := lockForUpdate(tx).Where("provider = ? AND event_id = ?", provider, eventId).First(&event).Error; err != nil {
		return event, err
	}
	if processedAt < event.ReceivedAt {
		return event, ErrSheJanePaidInvalid
	}
	if event.Status == SheJanePaymentProcessed {
		return event, nil
	}
	if event.Status != SheJanePaymentReceived {
		return event, ErrSheJanePaidTransition
	}
	event.Status = SheJanePaymentProcessed
	event.ProcessedAt = processedAt
	err := tx.Model(&event).Updates(map[string]any{
		"status":       event.Status,
		"processed_at": event.ProcessedAt,
	}).Error
	return event, err
}

func createSheJaneBalanceEntry(tx *gorm.DB, entry SheJaneBalanceEntry) error {
	if entry.UserId <= 0 || entry.BalanceAfter < 0 || entry.BalanceAfter > common.MaxQuota || !validSheJanePaidIdentifier(entry.IdempotencyKey) || !validSheJanePaidIdentifier(entry.ReferenceId) || entry.CreatedAt <= 0 {
		return ErrSheJanePaidInvalid
	}
	switch entry.Kind {
	case SheJaneBalanceReserve, SheJaneBalanceSettle, SheJaneBalanceRelease, SheJaneBalanceRefund, SheJaneBalanceAdjustment:
	default:
		return ErrSheJanePaidInvalid
	}
	entry.allowCreate = true
	created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
	if created.Error != nil {
		return created.Error
	}
	if created.RowsAffected != 1 {
		return ErrSheJanePaidConflict
	}
	return nil
}

func ensureSheJaneReservationKeyAvailable(tx *gorm.DB, column, key string, reservationId int64) error {
	var existing SheJaneBillingReservation
	err := tx.Where(column+" = ?", key).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.Id != reservationId {
		return ErrSheJanePaidConflict
	}
	return nil
}

func checkedSheJanePaidBalance(current, delta int) (int, error) {
	next := int64(current) + int64(delta)
	if next < 0 {
		return 0, ErrSheJanePaidInsufficientQuota
	}
	if next > int64(common.MaxQuota) {
		return 0, ErrSheJanePaidInvalid
	}
	return int(next), nil
}

func updateSheJanePaidQuotaWithTx(tx *gorm.DB, user *User, quota int) error {
	if tx == nil || user == nil || user.Id <= 0 || quota < 0 || quota > common.MaxQuota {
		return ErrSheJanePaidInvalid
	}
	currentVersion := user.QuotaVersion
	if currentVersion < 1 {
		currentVersion = 1
	}
	if currentVersion == math.MaxInt64 {
		return ErrUserQuotaVersionConflict
	}
	nextVersion := currentVersion + 1
	if err := setUserQuotaVersionFence(user.Id, nextVersion); err != nil {
		return err
	}
	result := tx.Model(&User{}).
		Where("id = ? AND quota_version = ?", user.Id, user.QuotaVersion).
		Updates(map[string]any{"quota": quota, "quota_version": nextVersion})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrUserQuotaVersionConflict
	}
	user.Quota = quota
	user.QuotaVersion = nextVersion
	return nil
}

func enableSheJanePaidWalletWithTx(tx *gorm.DB, user *User) error {
	if tx == nil || user == nil || user.Id <= 0 {
		return ErrSheJanePaidInvalid
	}
	nextVersion, err := IncrementUserQuotaVersionWithTx(tx, user.Id)
	if err != nil {
		return err
	}
	if err := tx.Model(user).Update("she_jane_paid_managed", true).Error; err != nil {
		return err
	}
	user.QuotaVersion = nextVersion
	user.SheJanePaidManaged = true
	return nil
}

func validSheJanePaidIdentifier(value string) bool {
	return sheJanePaidIdentifierPattern.MatchString(value)
}

func ensureSheJanePaidDatabaseGuards() error {
	switch {
	case common.UsingMainDatabase(common.DatabaseTypeSQLite):
		for _, statement := range []string{
			`CREATE TRIGGER IF NOT EXISTS she_jane_balance_entries_no_update BEFORE UPDATE ON she_jane_balance_entries BEGIN SELECT RAISE(ABORT, 'SheJane balance entries are immutable'); END`,
			`CREATE TRIGGER IF NOT EXISTS she_jane_balance_entries_no_delete BEFORE DELETE ON she_jane_balance_entries BEGIN SELECT RAISE(ABORT, 'SheJane balance entries are immutable'); END`,
			`CREATE TRIGGER IF NOT EXISTS she_jane_paid_quota_version_guard BEFORE UPDATE OF quota ON users WHEN OLD.she_jane_paid_managed = 1 AND NEW.quota <> OLD.quota AND NEW.quota_version <= OLD.quota_version BEGIN SELECT RAISE(ABORT, 'SheJane paid quota updates require quota_version advance'); END`,
		} {
			if err := DB.Exec(statement).Error; err != nil {
				return err
			}
		}
		for name, fragments := range map[string][3]string{
			"she_jane_balance_entries_no_update": {"BEFORE UPDATE ON she_jane_balance_entries", "RAISE(ABORT", "SheJane balance entries are immutable"},
			"she_jane_balance_entries_no_delete": {"BEFORE DELETE ON she_jane_balance_entries", "RAISE(ABORT", "SheJane balance entries are immutable"},
			"she_jane_paid_quota_version_guard":  {"BEFORE UPDATE OF quota ON users", "OLD.she_jane_paid_managed = 1 AND NEW.quota <> OLD.quota AND NEW.quota_version <= OLD.quota_version", "RAISE(ABORT"},
		} {
			var count int64
			if err := DB.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name = ? AND sql LIKE ? AND sql LIKE ? AND sql LIKE ?", name, "%"+fragments[0]+"%", "%"+fragments[1]+"%", "%"+fragments[2]+"%").Scan(&count).Error; err != nil || count != 1 {
				if err != nil {
					return err
				}
				return fmt.Errorf("invalid SheJane database trigger: %s", name)
			}
		}
		return nil
	case common.UsingMainDatabase(common.DatabaseTypeMySQL):
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		triggers := []struct {
			name, table, event, marker, guard, sql string
		}{
			{"she_jane_balance_entries_no_update", "she_jane_balance_entries", "UPDATE", "SheJane balance entries are immutable", "SIGNAL SQLSTATE", `CREATE TRIGGER she_jane_balance_entries_no_update BEFORE UPDATE ON she_jane_balance_entries FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'SheJane balance entries are immutable'`},
			{"she_jane_balance_entries_no_delete", "she_jane_balance_entries", "DELETE", "SheJane balance entries are immutable", "SIGNAL SQLSTATE", `CREATE TRIGGER she_jane_balance_entries_no_delete BEFORE DELETE ON she_jane_balance_entries FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'SheJane balance entries are immutable'`},
			{"she_jane_paid_quota_version_guard", "users", "UPDATE", "SheJane paid quota updates require quota_version advance", "NEW.quota_version <= OLD.quota_version", `CREATE TRIGGER she_jane_paid_quota_version_guard BEFORE UPDATE ON users FOR EACH ROW BEGIN IF OLD.she_jane_paid_managed = 1 AND NEW.quota <> OLD.quota AND NEW.quota_version <= OLD.quota_version THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'SheJane paid quota updates require quota_version advance'; END IF; END`},
		}
		for _, trigger := range triggers {
			var count int64
			check := "SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = DATABASE() AND TRIGGER_NAME = ? AND EVENT_OBJECT_TABLE = ? AND ACTION_TIMING = 'BEFORE' AND EVENT_MANIPULATION = ? AND ACTION_STATEMENT LIKE ? AND ACTION_STATEMENT LIKE ?"
			if err := DB.Raw(check, trigger.name, trigger.table, trigger.event, "%"+trigger.marker+"%", "%"+trigger.guard+"%").Scan(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				continue
			}
			// MySQL rejects CREATE TRIGGER through the prepared statement protocol.
			if _, err := sqlDB.Exec(trigger.sql); err != nil {
				if checkErr := DB.Raw(check, trigger.name, trigger.table, trigger.event, "%"+trigger.marker+"%", "%"+trigger.guard+"%").Scan(&count).Error; checkErr != nil || count != 1 {
					return err
				}
			}
		}
		return nil
	case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
		for _, statement := range []string{
			`CREATE OR REPLACE FUNCTION she_jane_balance_entries_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'SheJane balance entries are immutable'; END $$`,
			`CREATE OR REPLACE FUNCTION she_jane_paid_quota_version_guard_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.she_jane_paid_managed AND NEW.quota <> OLD.quota AND NEW.quota_version <= OLD.quota_version THEN RAISE EXCEPTION 'SheJane paid quota updates require quota_version advance'; END IF; RETURN NEW; END $$`,
			`DO $$ BEGIN CREATE TRIGGER she_jane_balance_entries_no_update BEFORE UPDATE ON she_jane_balance_entries FOR EACH ROW EXECUTE PROCEDURE she_jane_balance_entries_immutable(); EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
			`DO $$ BEGIN CREATE TRIGGER she_jane_balance_entries_no_delete BEFORE DELETE ON she_jane_balance_entries FOR EACH ROW EXECUTE PROCEDURE she_jane_balance_entries_immutable(); EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
			`DO $$ BEGIN CREATE TRIGGER she_jane_paid_quota_version_guard BEFORE UPDATE OF quota ON users FOR EACH ROW EXECUTE PROCEDURE she_jane_paid_quota_version_guard_fn(); EXCEPTION WHEN duplicate_object THEN NULL; END $$`,
		} {
			if err := DB.Exec(statement).Error; err != nil {
				return err
			}
		}
		checks := []struct{ name, table, event, function string }{
			{"she_jane_balance_entries_no_update", "she_jane_balance_entries", "UPDATE", "she_jane_balance_entries_immutable"},
			{"she_jane_balance_entries_no_delete", "she_jane_balance_entries", "DELETE", "she_jane_balance_entries_immutable"},
			{"she_jane_paid_quota_version_guard", "users", "UPDATE", "she_jane_paid_quota_version_guard_fn"},
		}
		for _, check := range checks {
			var count int64
			query := `SELECT COUNT(*) FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid JOIN pg_namespace n ON n.oid = c.relnamespace JOIN pg_proc p ON p.oid = t.tgfoid JOIN pg_namespace pn ON pn.oid = p.pronamespace WHERE NOT t.tgisinternal AND t.tgenabled IN ('O', 'A') AND n.nspname = current_schema() AND pn.nspname = current_schema() AND t.tgname = ? AND c.relname = ? AND p.proname = ? AND pg_get_triggerdef(t.oid) LIKE ?`
			if err := DB.Raw(query, check.name, check.table, check.function, "%BEFORE "+check.event+"%").Scan(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("invalid SheJane database trigger: %s", check.name)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported database for SheJane paid-operation guards")
	}
}
