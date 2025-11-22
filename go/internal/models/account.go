package models

import (
	"time"
)

// Account represents a user account with OAuth tokens
type Account struct {
	AccountID     string           `json:"accountId"`
	Email         string           `json:"email"`
	Name          string           `json:"name"`
	AccessToken   string           `json:"access_token"`
	RefreshToken  string           `json:"refresh_token"`
	ExpiresIn     int              `json:"expires_in"`
	Timestamp     int64            `json:"timestamp"`
	Enable        bool             `json:"enable"`
	Models        map[string]Model `json:"models,omitempty"`
	LastRefresh   int64            `json:"lastRefresh,omitempty"`
	RefreshStatus string           `json:"refreshStatus,omitempty"`
	Usage         *UsageStats      `json:"usage,omitempty"`
	ErrorTracking *ErrorTracking   `json:"errorTracking,omitempty"`
}

// Model represents an AI model
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// UsageStats tracks account usage
type UsageStats struct {
	TotalTokens  int64  `json:"totalTokens"`
	InputTokens  int64  `json:"inputTokens"`
	OutputTokens int64  `json:"outputTokens"`
	RequestCount int64  `json:"requestCount"`
	LastUsed     *int64 `json:"lastUsed,omitempty"`
}

// ErrorTracking tracks account errors
type ErrorTracking struct {
	ConsecutiveFailures int    `json:"consecutiveFailures"`
	LastError           string `json:"lastError,omitempty"`
	LastErrorTime       *int64 `json:"lastErrorTime,omitempty"`
	FailedUntil         *int64 `json:"failedUntil,omitempty"`
}

// IsExpired checks if the access token is expired
func (a *Account) IsExpired() bool {
	if a.Timestamp == 0 || a.ExpiresIn == 0 {
		return true
	}
	expiryTime := time.Unix(a.Timestamp/1000, 0).Add(time.Duration(a.ExpiresIn) * time.Second)
	return time.Now().After(expiryTime)
}

// IsInCooldown checks if account is in error cooldown
func (a *Account) IsInCooldown() bool {
	if a.ErrorTracking == nil || a.ErrorTracking.FailedUntil == nil {
		return false
	}
	return time.Now().Unix() < *a.ErrorTracking.FailedUntil
}

// NeedsRefresh checks if account needs token refresh
func (a *Account) NeedsRefresh() bool {
	// 如果禁用或在冷却期，不刷新
	if !a.Enable || a.IsInCooldown() {
		return false
	}
	// 如果还有30分钟就过期，需要刷新
	if a.Timestamp == 0 || a.ExpiresIn == 0 {
		return true
	}
	expiryTime := time.Unix(a.Timestamp/1000, 0).Add(time.Duration(a.ExpiresIn) * time.Second)
	return time.Until(expiryTime) < 30*time.Minute
}
