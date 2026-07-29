package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrUserQuotaCachePending = errors.New("user quota update is pending")

var ErrUserQuotaCacheBypass = errors.New("user quota cache bypassed for paid wallet")

var ErrUserQuotaVersionConflict = errors.New("user quota version update conflicted")

func getUserQuotaFenceKey(userId int) string {
	return fmt.Sprintf("quota:user:fence:%d", userId)
}

func getUserQuotaVersionKey(userId int) string {
	return fmt.Sprintf("quota:user:version:%d", userId)
}

func getUserQuotaVersionFloor(userId int) (int64, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	values, err := common.RDB.MGet(context.Background(), getUserQuotaFenceKey(userId), getUserQuotaVersionKey(userId)).Result()
	if err != nil {
		return 0, err
	}
	var floor int64
	for _, value := range values {
		if value == nil {
			continue
		}
		parsed, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		if err != nil {
			return 0, err
		}
		if parsed > floor {
			floor = parsed
		}
	}
	return floor, nil
}

func setUserQuotaVersionFence(userId int, quotaVersion int64) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 || quotaVersion <= 0 {
		return fmt.Errorf("invalid user quota fence")
	}
	const script = `
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local incoming = tonumber(ARGV[1])
if current < incoming then
  redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
elseif current == incoming then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
elseif redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return 1`
	return common.RDB.Eval(context.Background(), script, []string{getUserQuotaFenceKey(userId)}, quotaVersion, userAuthFenceTTLSeconds()).Err()
}

func writeUserQuotaCacheAtVersion(userId, quota int, quotaVersion int64) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 || quota < 0 || quota > common.MaxQuota || quotaVersion <= 0 {
		return fmt.Errorf("invalid user quota cache state")
	}
	const script = `
local incoming = tonumber(ARGV[1])
local pending = tonumber(redis.call('GET', KEYS[2]) or '0')
local committed = tonumber(redis.call('GET', KEYS[3]) or '0')
local current = tonumber(redis.call('HGET', KEYS[1], 'QuotaVersion') or '0')
if pending > incoming or committed > incoming or current > incoming then
  return 0
end
if committed < incoming then
  redis.call('SET', KEYS[3], ARGV[1])
end
if pending > 0 and pending <= incoming then
  redis.call('DEL', KEYS[2])
end
if redis.call('EXISTS', KEYS[1]) == 0 then
  return 1
end
redis.call('HSET', KEYS[1], 'Quota', ARGV[2], 'QuotaVersion', ARGV[1], 'CacheSchema', ARGV[3])
redis.call('EXPIRE', KEYS[1], ARGV[4])
return 1`
	result, err := common.RDB.Eval(context.Background(), script,
		[]string{getUserCacheKey(userId), getUserQuotaFenceKey(userId), getUserQuotaVersionKey(userId)},
		quotaVersion, quota, userCacheSchemaVersion, userCacheTTLSeconds(),
	).Int()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrUserQuotaCachePending
	}
	return nil
}

func IncrementUserQuotaVersionWithTx(tx *gorm.DB, userId int) (int64, error) {
	if tx == nil || userId <= 0 {
		return 0, fmt.Errorf("invalid user quota version update")
	}
	for range 3 {
		var user User
		if err := lockForUpdate(tx).Select("id", "quota_version").Where("id = ?", userId).First(&user).Error; err != nil {
			return 0, err
		}
		current := user.QuotaVersion
		if current < 1 {
			current = 1
		}
		next := current + 1
		if err := setUserQuotaVersionFence(userId, next); err != nil {
			return 0, err
		}
		result := tx.Model(&User{}).
			Where("id = ? AND quota_version = ?", userId, user.QuotaVersion).
			Update("quota_version", next)
		if result.Error != nil {
			return 0, result.Error
		}
		if result.RowsAffected == 1 {
			return next, nil
		}
	}
	return 0, ErrUserQuotaVersionConflict
}

func PublishUserQuotaCache(userId int) error {
	user, err := GetUserById(userId, false)
	if err != nil {
		return err
	}
	return populateUserCache(*user)
}

func InitializeUserQuotaVersions() error {
	return DB.Model(&User{}).Where("quota_version IS NULL OR quota_version < ?", 1).Update("quota_version", 1).Error
}
