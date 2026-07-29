package model

import "gorm.io/gorm"

type SheJaneTelemetryToken struct {
	Id             int64  `json:"-" gorm:"primaryKey"`
	UserId         int    `json:"-" gorm:"not null;index"`
	DeviceId       int64  `json:"-" gorm:"not null;index"`
	ActiveDeviceId *int64 `json:"-" gorm:"uniqueIndex:idx_she_jane_telemetry_active_device"`
	TokenHash      string `json:"-" gorm:"type:varchar(64);not null;uniqueIndex"`
	CreatedAt      int64  `json:"-" gorm:"type:bigint;not null"`
	ExpiresAt      int64  `json:"-" gorm:"type:bigint;not null;index"`
	RevokedAt      int64  `json:"-" gorm:"type:bigint;not null"`
}

func (SheJaneTelemetryToken) TableName() string {
	return "she_jane_telemetry_tokens"
}

func CreateSheJaneTelemetryTokenWithTx(tx *gorm.DB, token *SheJaneTelemetryToken) error {
	if tx == nil || token == nil || token.UserId <= 0 || token.DeviceId <= 0 || token.TokenHash == "" || token.CreatedAt <= 0 || token.ExpiresAt <= token.CreatedAt {
		return gorm.ErrInvalidData
	}
	token.ActiveDeviceId = &token.DeviceId
	if err := tx.Model(&SheJaneTelemetryToken{}).
		Where("user_id = ? AND device_id = ? AND revoked_at = 0", token.UserId, token.DeviceId).
		Updates(map[string]any{"revoked_at": token.CreatedAt, "active_device_id": nil}).Error; err != nil {
		return err
	}
	return tx.Create(token).Error
}

func GetActiveSheJaneTelemetryToken(tokenHash string, now int64) (*SheJaneTelemetryToken, error) {
	if tokenHash == "" || now <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var token SheJaneTelemetryToken
	err := DB.Table("she_jane_telemetry_tokens AS telemetry").
		Select("telemetry.*").
		Joins("JOIN she_jane_devices AS device ON device.id = telemetry.device_id AND device.user_id = telemetry.user_id").
		Where("telemetry.token_hash = ? AND telemetry.revoked_at = 0 AND telemetry.expires_at > ? AND device.revoked_at = 0", tokenHash, now).
		Take(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func RevokeSheJaneTelemetryTokensWithTx(tx *gorm.DB, userId int, deviceId int64, revokedAt int64) error {
	if tx == nil || userId <= 0 || deviceId <= 0 || revokedAt <= 0 {
		return gorm.ErrInvalidData
	}
	return tx.Model(&SheJaneTelemetryToken{}).
		Where("user_id = ? AND device_id = ? AND revoked_at = 0", userId, deviceId).
		Update("revoked_at", revokedAt).Error
}
