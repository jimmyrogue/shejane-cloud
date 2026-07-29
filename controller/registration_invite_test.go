package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRegistrationInviterEnforcesInviteOnlyMode(t *testing.T) {
	setupSheJaneControllerTest(t)
	previous := common.InviteRegisterEnabled
	t.Cleanup(func() { common.InviteRegisterEnabled = previous })

	common.InviteRegisterEnabled = false
	inviterID, ok := resolveRegistrationInviter("")
	assert.True(t, ok)
	assert.Zero(t, inviterID)

	common.InviteRegisterEnabled = true
	_, ok = resolveRegistrationInviter("")
	assert.False(t, ok)
	_, ok = resolveRegistrationInviter("unknown")
	assert.False(t, ok)

	inviterID, ok = resolveRegistrationInviter("controller-shejane-aff")
	require.True(t, ok)
	assert.Positive(t, inviterID)
}
