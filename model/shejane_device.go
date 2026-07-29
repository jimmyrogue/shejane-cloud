package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type SheJaneDevice struct {
	Id         int64  `json:"id" gorm:"primaryKey"`
	UserId     int    `json:"-" gorm:"not null;index"`
	TokenId    int    `json:"-" gorm:"not null;uniqueIndex"`
	ClientId   string `json:"client_id" gorm:"type:varchar(32);not null"`
	Name       string `json:"name" gorm:"type:varchar(80);not null"`
	Platform   string `json:"platform" gorm:"type:varchar(16);not null"`
	AppVersion string `json:"app_version" gorm:"type:varchar(32);not null"`
	CreatedAt  int64  `json:"created_at" gorm:"type:bigint;not null"`
	RevokedAt  int64  `json:"revoked_at" gorm:"type:bigint;not null"`
}

func (SheJaneDevice) TableName() string {
	return "she_jane_devices"
}

func CreateSheJaneDeviceWithTx(tx *gorm.DB, device *SheJaneDevice) error {
	if tx == nil || device == nil || device.UserId <= 0 || device.TokenId <= 0 {
		return gorm.ErrInvalidData
	}
	return tx.Create(device).Error
}

func LockSheJaneAuthorizationIdentityWithTx(tx *gorm.DB, userId int, sessionId string) (*User, *UserSession, error) {
	if tx == nil || userId <= 0 || sessionId == "" {
		return nil, nil, gorm.ErrInvalidData
	}
	var user User
	if err := lockForUpdate(tx).Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, nil, err
	}
	var session UserSession
	if err := lockForUpdate(tx).Where("sid = ? AND user_id = ?", sessionId, userId).First(&session).Error; err != nil {
		return nil, nil, err
	}
	return &user, &session, nil
}

func CountUserTokensWithTx(tx *gorm.DB, userId int) (int64, error) {
	var count int64
	err := tx.Model(&Token{}).Where("user_id = ?", userId).Count(&count).Error
	return count, err
}

func CreateSheJaneManagedCredentialWithTx(tx *gorm.DB, token *Token, device *SheJaneDevice) error {
	if tx == nil || token == nil || device == nil || token.UserId <= 0 || device.UserId != token.UserId {
		return gorm.ErrInvalidData
	}
	if err := tx.Create(token).Error; err != nil {
		return err
	}
	device.TokenId = token.Id
	return CreateSheJaneDeviceWithTx(tx, device)
}

func ListSheJaneDevices(userId int) ([]SheJaneDevice, error) {
	var devices []SheJaneDevice
	err := DB.Select("id", "client_id", "name", "platform", "app_version", "created_at", "revoked_at").
		Where("user_id = ?", userId).
		Order("id DESC").
		Find(&devices).Error
	return devices, err
}

func IsSheJaneManagedToken(userId, tokenId int) (bool, error) {
	if userId <= 0 || tokenId <= 0 {
		return false, nil
	}
	var count int64
	err := DB.Model(&SheJaneDevice{}).
		Where("user_id = ? AND token_id = ? AND revoked_at = 0", userId, tokenId).
		Count(&count).Error
	return count > 0, err
}

func AnySheJaneManagedToken(userId int, tokenIds []int) (bool, error) {
	if userId <= 0 || len(tokenIds) == 0 {
		return false, nil
	}
	var count int64
	err := DB.Model(&SheJaneDevice{}).
		Where("user_id = ? AND token_id IN ? AND revoked_at = 0", userId, tokenIds).
		Count(&count).Error
	return count > 0, err
}

func GetOwnedSheJaneDeviceAndToken(userId int, deviceId int64) (*SheJaneDevice, *Token, error) {
	if userId <= 0 || deviceId <= 0 {
		return nil, nil, gorm.ErrRecordNotFound
	}
	var device SheJaneDevice
	if err := DB.Where("id = ? AND user_id = ?", deviceId, userId).First(&device).Error; err != nil {
		return nil, nil, err
	}
	var token Token
	if err := DB.Unscoped().Where("id = ? AND user_id = ?", device.TokenId, userId).First(&token).Error; err != nil {
		return nil, nil, err
	}
	return &device, &token, nil
}

func RevokeSheJaneManagedCredentialWithTx(tx *gorm.DB, userId int, deviceId int64, revokedAt int64) (*SheJaneDevice, bool, error) {
	if tx == nil || userId <= 0 || deviceId <= 0 || revokedAt <= 0 {
		return nil, false, gorm.ErrInvalidData
	}
	var device SheJaneDevice
	if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", deviceId, userId).First(&device).Error; err != nil {
		return nil, false, err
	}
	if device.RevokedAt != 0 {
		return &device, true, nil
	}
	var token Token
	if err := lockForUpdate(tx.Unscoped()).Where("id = ? AND user_id = ?", device.TokenId, userId).First(&token).Error; err != nil {
		return nil, false, err
	}
	result := tx.Model(&SheJaneDevice{}).
		Where("id = ? AND user_id = ? AND revoked_at = 0", deviceId, userId).
		Update("revoked_at", revokedAt)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, false, gorm.ErrRecordNotFound
	}
	if err := RevokeSheJaneTelemetryTokensWithTx(tx, userId, deviceId, revokedAt); err != nil {
		return nil, false, err
	}
	if err := tx.Unscoped().Model(&Token{}).
		Where("id = ? AND user_id = ?", token.Id, userId).
		Update("status", common.TokenStatusDisabled).Error; err != nil {
		return nil, false, err
	}
	if err := tx.Delete(&token).Error; err != nil {
		return nil, false, err
	}
	device.RevokedAt = revokedAt
	return &device, false, nil
}
