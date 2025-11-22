package oauth

import (
	"os"
	"testing"
	"time"

	"github.com/antigravity/api-proxy/internal/models"
	"github.com/antigravity/api-proxy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestClient(t *testing.T) (*Client, string) {
	// Create temp dir for accounts
	tmpDir, err := os.MkdirTemp("", "oauth_test")
	require.NoError(t, err)

	logger := zap.NewNop()
	client := NewClient(8080, tmpDir, logger)

	return client, tmpDir
}

func createTestAccount(t *testing.T, store *storage.AccountStore, id string, enable bool, cooldown bool) {
	account := &models.Account{
		AccountID:   id,
		Email:       id + "@example.com",
		Enable:      enable,
		AccessToken: "valid_token",
		ExpiresIn:   3600,
		Timestamp:   time.Now().UnixMilli(),
	}

	if cooldown {
		failedUntil := time.Now().Add(1 * time.Hour).UnixMilli()
		account.ErrorTracking = &models.ErrorTracking{
			FailedUntil: &failedUntil,
		}
	}

	err := store.Save(account)
	require.NoError(t, err)
}

func TestGetToken_Rotation(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	store := client.AccountStore()

	// Create 3 accounts
	createTestAccount(t, store, "acc1", true, false)
	createTestAccount(t, store, "acc2", true, false)
	createTestAccount(t, store, "acc3", true, false)

	// First call should get one of them (order depends on file system, but rotation should work)
	acc1, err := client.GetToken()
	require.NoError(t, err)
	assert.NotNil(t, acc1)

	// Second call should get next
	acc2, err := client.GetToken()
	require.NoError(t, err)
	assert.NotNil(t, acc2)
	assert.NotEqual(t, acc1.AccountID, acc2.AccountID)

	// Third call should get the last one
	acc3, err := client.GetToken()
	require.NoError(t, err)
	assert.NotNil(t, acc3)
	assert.NotEqual(t, acc1.AccountID, acc3.AccountID)
	assert.NotEqual(t, acc2.AccountID, acc3.AccountID)

	// Fourth call should wrap around
	acc4, err := client.GetToken()
	require.NoError(t, err)
	assert.Equal(t, acc1.AccountID, acc4.AccountID)
}

func TestGetToken_SkipDisabled(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	store := client.AccountStore()

	createTestAccount(t, store, "acc1", false, false) // Disabled
	createTestAccount(t, store, "acc2", true, false)  // Enabled

	// Should skip acc1 and get acc2
	acc, err := client.GetToken()
	require.NoError(t, err)
	assert.Equal(t, "acc2", acc.AccountID)

	// Should still get acc2 on next call
	acc, err = client.GetToken()
	require.NoError(t, err)
	assert.Equal(t, "acc2", acc.AccountID)
}

func TestGetToken_SkipCooldown(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	store := client.AccountStore()

	createTestAccount(t, store, "acc1", true, true)  // Cooldown
	createTestAccount(t, store, "acc2", true, false) // Enabled

	// Should skip acc1 and get acc2
	acc, err := client.GetToken()
	require.NoError(t, err)
	assert.Equal(t, "acc2", acc.AccountID)
}

func TestGetToken_NoAvailable(t *testing.T) {
	client, tmpDir := setupTestClient(t)
	defer os.RemoveAll(tmpDir)

	store := client.AccountStore()

	createTestAccount(t, store, "acc1", false, false) // Disabled
	createTestAccount(t, store, "acc2", true, true)   // Cooldown

	// Should return error
	acc, err := client.GetToken()
	assert.Error(t, err)
	assert.Nil(t, acc)
	assert.Contains(t, err.Error(), "no valid accounts available")
}
