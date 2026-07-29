package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSheJaneDeviceMarksAndListsOwnedManagedTokens(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&SheJaneDevice{}))
	require.NoError(t, DB.Exec("DELETE FROM she_jane_devices").Error)
	t.Cleanup(func() { _ = DB.Exec("DELETE FROM she_jane_devices").Error })

	device := SheJaneDevice{
		UserId: 42, TokenId: 7, ClientId: "shejane-desktop", Name: "Jimmy's Mac",
		Platform: "macos", AppVersion: "0.1.8", CreatedAt: 100,
	}
	require.NoError(t, CreateSheJaneDeviceWithTx(DB, &device))
	assert.Positive(t, device.Id)

	managed, err := IsSheJaneManagedToken(42, 7)
	require.NoError(t, err)
	assert.True(t, managed)
	managed, err = IsSheJaneManagedToken(41, 7)
	require.NoError(t, err)
	assert.False(t, managed)

	devices, err := ListSheJaneDevices(42)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, device.Id, devices[0].Id)
	assert.Equal(t, 0, devices[0].TokenId, "the list contract must not expose Token IDs")
}

func TestSheJaneTokenDenyFenceRejectsCachedAndStaleCredentials(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	token := Token{
		UserId: 42, Key: "managed-cache-key", Status: common.TokenStatusEnabled,
		Name: "managed", ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, DB.Create(&token).Error)
	require.NoError(t, cacheSetToken(token))

	require.NoError(t, WriteSheJaneTokenDenyFence(token.Key))
	err := cacheSetToken(token)
	assert.ErrorIs(t, err, ErrSheJaneTokenCacheDenied)

	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, common.TokenStatusDisabled, cached.Status)
	_, err = ValidateUserToken(token.Key)
	assert.True(t, errors.Is(err, ErrTokenInvalid))
}
