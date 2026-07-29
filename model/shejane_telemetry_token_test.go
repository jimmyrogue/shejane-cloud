package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSheJaneTelemetryTokenEnforcesOneActiveCredentialPerDevice(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&SheJaneTelemetryToken{}))
	deviceID := int64(42)
	first := SheJaneTelemetryToken{
		UserId: 1, DeviceId: deviceID, ActiveDeviceId: &deviceID,
		TokenHash: "first", CreatedAt: 100, ExpiresAt: 200,
	}
	second := SheJaneTelemetryToken{
		UserId: 1, DeviceId: deviceID, ActiveDeviceId: &deviceID,
		TokenHash: "second", CreatedAt: 101, ExpiresAt: 201,
	}

	require.NoError(t, DB.Create(&first).Error)
	assert.Error(t, DB.Create(&second).Error)
}
