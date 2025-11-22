package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/antigravity/api-proxy/internal/models"
)

// AccountStore handles account persistence
type AccountStore struct {
	accountsDir string
}

// NewAccountStore creates a new account store
func NewAccountStore(accountsDir string) *AccountStore {
	return &AccountStore{
		accountsDir: accountsDir,
	}
}

// Save saves an account to file
func (s *AccountStore) Save(account *models.Account) error {
	// 确保目录存在
	if err := os.MkdirAll(s.accountsDir, 0755); err != nil {
		return fmt.Errorf("failed to create accounts directory: %w", err)
	}

	// 构建文件路径
	filename := account.AccountID + ".json"
	filePath := filepath.Join(s.accountsDir, filename)

	// 序列化账号数据
	data, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal account: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write account file: %w", err)
	}

	return nil
}

// Load loads an account from file
func (s *AccountStore) Load(accountID string) (*models.Account, error) {
	filename := accountID + ".json"
	filePath := filepath.Join(s.accountsDir, filename)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read account file: %w", err)
	}

	var account models.Account
	if err := json.Unmarshal(data, &account); err != nil {
		return nil, fmt.Errorf("failed to unmarshal account: %w", err)
	}

	return &account, nil
}

// List lists all account IDs
func (s *AccountStore) List() ([]string, error) {
	entries, err := os.ReadDir(s.accountsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read accounts directory: %w", err)
	}

	var accountIDs []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			accountID := entry.Name()[:len(entry.Name())-5] // 移除 .json
			accountIDs = append(accountIDs, accountID)
		}
	}

	return accountIDs, nil
}

// Delete deletes an account file
func (s *AccountStore) Delete(accountID string) error {
	filename := accountID + ".json"
	filePath := filepath.Join(s.accountsDir, filename)
	return os.Remove(filePath)
}
