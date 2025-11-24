package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/antigravity/api-proxy/internal/models"
)

// KeyStore handles API key persistence
type KeyStore struct {
	keysDir string
}

// NewKeyStore creates a new key store
func NewKeyStore(keysDir string) *KeyStore {
	return &KeyStore{
		keysDir: keysDir,
	}
}

// Save saves an API key to file
func (s *KeyStore) Save(key *models.APIKey) error {
	// Ensure directory exists
	if err := os.MkdirAll(s.keysDir, 0755); err != nil {
		return fmt.Errorf("failed to create keys directory: %w", err)
	}

	// Build file path (use key as filename)
	filename := sanitizeKeyFilename(key.Key) + ".json"
	filePath := filepath.Join(s.keysDir, filename)

	// Serialize key data
	data, err := json.MarshalIndent(key, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal key: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	return nil
}

// Load loads an API key from file
func (s *KeyStore) Load(key string) (*models.APIKey, error) {
	filename := sanitizeKeyFilename(key) + ".json"
	filePath := filepath.Join(s.keysDir, filename)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	var apiKey models.APIKey
	if err := json.Unmarshal(data, &apiKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal key: %w", err)
	}

	return &apiKey, nil
}

// List lists all API keys
func (s *KeyStore) List() ([]*models.APIKey, error) {
	entries, err := os.ReadDir(s.keysDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*models.APIKey{}, nil
		}
		return nil, fmt.Errorf("failed to read keys directory: %w", err)
	}

	var keys []*models.APIKey
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			filePath := filepath.Join(s.keysDir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			var key models.APIKey
			if err := json.Unmarshal(data, &key); err != nil {
				continue
			}

			keys = append(keys, &key)
		}
	}

	return keys, nil
}

// Delete deletes an API key file
func (s *KeyStore) Delete(key string) error {
	// Reject keys with dangerous path characters
	if strings.Contains(key, "/") || strings.Contains(key, "\\") || strings.Contains(key, "..") {
		return fmt.Errorf("invalid key format")
	}
	filename := sanitizeKeyFilename(key) + ".json"
	filePath := filepath.Join(s.keysDir, filename)
	return os.Remove(filePath)
}

// Exists checks if a key exists
func (s *KeyStore) Exists(key string) bool {
	filename := sanitizeKeyFilename(key) + ".json"
	filePath := filepath.Join(s.keysDir, filename)
	_, err := os.Stat(filePath)
	return err == nil
}

// sanitizeKeyFilename converts a key to a safe filename
func sanitizeKeyFilename(key string) string {
	// Replace special characters
	return strings.ReplaceAll(key, ":", "_")
}

// UsageStore handles usage statistics persistence
type UsageStore struct {
	usageDir string
}

// NewUsageStore creates a new usage store
func NewUsageStore(usageDir string) *UsageStore {
	return &UsageStore{
		usageDir: usageDir,
	}
}

// UsageRecord represents a usage record
type UsageRecord struct {
	Date         string `json:"date"`          // YYYY-MM-DD
	AccountID    string `json:"account_id"`
	TotalTokens  int64  `json:"total_tokens"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	RequestCount int64  `json:"request_count"`
}

// RecordUsage records usage for an account
func (s *UsageStore) RecordUsage(accountID string, inputTokens, outputTokens int64) error {
	// Ensure directory exists
	if err := os.MkdirAll(s.usageDir, 0755); err != nil {
		return fmt.Errorf("failed to create usage directory: %w", err)
	}

	// Get today's date
	today := time.Now().Format("2006-01-02")
	
	// Build file path for today
	filename := fmt.Sprintf("%s_%s.json", today, accountID)
	filePath := filepath.Join(s.usageDir, filename)

	// Try to load existing record
	var record UsageRecord
	data, err := os.ReadFile(filePath)
	if err == nil {
		json.Unmarshal(data, &record)
	} else {
		// Create new record
		record = UsageRecord{
			Date:      today,
			AccountID: accountID,
		}
	}

	// Update record
	record.InputTokens += inputTokens
	record.OutputTokens += outputTokens
	record.TotalTokens += inputTokens + outputTokens
	record.RequestCount++

	// Save record
	data, err = json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal usage record: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write usage file: %w", err)
	}

	return nil
}

// GetUsageHistory gets usage history for a date range
func (s *UsageStore) GetUsageHistory(days int) ([]UsageRecord, error) {
	entries, err := os.ReadDir(s.usageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []UsageRecord{}, nil
		}
		return nil, fmt.Errorf("failed to read usage directory: %w", err)
	}

	var records []UsageRecord
	cutoffDate := time.Now().AddDate(0, 0, -days)

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			// Parse date from filename (YYYY-MM-DD_accountid.json)
			parts := strings.Split(entry.Name(), "_")
			if len(parts) < 2 {
				continue
			}

			dateStr := parts[0]
			recordDate, err := time.Parse("2006-01-02", dateStr)
			if err != nil || recordDate.Before(cutoffDate) {
				continue
			}

			filePath := filepath.Join(s.usageDir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			var record UsageRecord
			if err := json.Unmarshal(data, &record); err != nil {
				continue
			}

			records = append(records, record)
		}
	}

	return records, nil
}
