package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/go-redis/redis/v8"
)

var ErrSheJaneTokenCacheDenied = errors.New("SheJane managed token is denied by cache fence")

func tokenCacheKey(key string) string {
	return fmt.Sprintf("token:%s", common.GenerateHMAC(key))
}

func sheJaneTokenDenyFenceKey(key string) string {
	return fmt.Sprintf("token:%s:shejane-deny", common.GenerateHMAC(key))
}

func cacheSetToken(token Token) error {
	if !common.RedisEnabled {
		return nil
	}
	cacheKey := tokenCacheKey(token.Key)
	fenceKey := sheJaneTokenDenyFenceKey(token.Key)
	token.Clean()
	ttl := time.Duration(common.RedisKeyCacheSeconds()) * time.Second
	if ttl <= 0 {
		ttl = time.Minute
	}
	allowIPs := ""
	if token.AllowIps != nil {
		allowIPs = *token.AllowIps
	}
	fields := map[string]interface{}{
		"Id": token.Id, "UserId": token.UserId, "Key": token.Key, "Status": token.Status,
		"Name": token.Name, "CreatedTime": token.CreatedTime, "AccessedTime": token.AccessedTime,
		"ExpiredTime": token.ExpiredTime, "RemainQuota": token.RemainQuota,
		"UnlimitedQuota": token.UnlimitedQuota, "ModelLimitsEnabled": token.ModelLimitsEnabled,
		"ModelLimits": token.ModelLimits, "AllowIps": allowIPs, "UsedQuota": token.UsedQuota,
		"Group": token.Group, "CrossGroupRetry": token.CrossGroupRetry,
	}
	ctx := context.Background()
	err := common.RDB.Watch(ctx, func(tx *redis.Tx) error {
		fenced, err := tx.Exists(ctx, fenceKey).Result()
		if err != nil {
			return err
		}
		if fenced != 0 {
			return ErrSheJaneTokenCacheDenied
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.HSet(ctx, cacheKey, fields)
			pipe.Expire(ctx, cacheKey, ttl)
			return nil
		})
		return err
	}, fenceKey)
	if errors.Is(err, redis.TxFailedErr) {
		return ErrSheJaneTokenCacheDenied
	}
	return err
}

func cacheDeleteToken(key string) error {
	err := common.RedisDelKey(tokenCacheKey(key))
	if err != nil {
		return err
	}
	return nil
}

func cacheIncrTokenQuota(key string, increment int64) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHIncrBy(fmt.Sprintf("token:%s", key), constant.TokenFiledRemainQuota, increment)
	if err != nil {
		return err
	}
	return nil
}

func cacheDecrTokenQuota(key string, decrement int64) error {
	return cacheIncrTokenQuota(key, -decrement)
}

func cacheSetTokenField(key string, field string, value string) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHSetField(fmt.Sprintf("token:%s", key), field, value)
	if err != nil {
		return err
	}
	return nil
}

// CacheGetTokenByKey 从缓存中获取 token，如果缓存中不存在，则从数据库中获取
func cacheGetTokenByKey(key string) (*Token, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	fenced, err := common.RDB.Exists(context.Background(), sheJaneTokenDenyFenceKey(key)).Result()
	if err != nil {
		return nil, err
	}
	if fenced != 0 {
		return &Token{Key: key, Status: common.TokenStatusDisabled}, nil
	}
	var token Token
	err = common.RedisHGetObj(tokenCacheKey(key), &token)
	if err != nil {
		return nil, err
	}
	token.Key = key
	return &token, nil
}

func WriteSheJaneTokenDenyFence(key string) error {
	if !common.RedisEnabled {
		return nil
	}
	if key == "" {
		return errors.New("token key is empty")
	}
	ttl := time.Duration(common.RedisKeyCacheSeconds()) * time.Second
	if ttl <= 0 {
		ttl = time.Minute
	}
	return common.RDB.Set(context.Background(), sheJaneTokenDenyFenceKey(key), "1", ttl).Err()
}
