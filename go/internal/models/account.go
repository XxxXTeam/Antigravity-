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
	RateLimitCount      int    `json:"rateLimitCount,omitempty"`
	RateLimitBackoff    int64  `json:"rateLimitBackoff,omitempty"`
	IsPermissionDenied  bool   `json:"isPermissionDenied,omitempty"`
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

// RecordSuccess updates account status on successful operation
func (a *Account) RecordSuccess() {
	a.RefreshStatus = "success"
	a.LastRefresh = time.Now().UnixMilli()
	if a.ErrorTracking == nil {
		a.ErrorTracking = &ErrorTracking{}
	}
	a.ErrorTracking.ConsecutiveFailures = 0
	a.ErrorTracking.LastError = ""
	a.ErrorTracking.LastErrorTime = nil
	a.ErrorTracking.FailedUntil = nil
	// Reset rate limit tracking on success
	a.ErrorTracking.RateLimitCount = 0
	a.ErrorTracking.RateLimitBackoff = 0
}

// RecordFailure updates account status on failed operation
func (a *Account) RecordFailure(err string) {
	a.RefreshStatus = "failed"
	if a.ErrorTracking == nil {
		a.ErrorTracking = &ErrorTracking{}
	}
	a.ErrorTracking.ConsecutiveFailures++
	a.ErrorTracking.LastError = err
	now := time.Now().Unix()
	a.ErrorTracking.LastErrorTime = &now

	// Calculate cooldown: 2^failures seconds, max 1 hour
	cooldownSeconds := int64(1 << a.ErrorTracking.ConsecutiveFailures)
	if cooldownSeconds > 3600 {
		cooldownSeconds = 3600
	}
	failedUntil := now + cooldownSeconds
	a.ErrorTracking.FailedUntil = &failedUntil
}

// RecordRateLimit handles 429 rate limit errors with adaptive backoff
func (a *Account) RecordRateLimit() {
	a.RefreshStatus = "rate_limited"
	if a.ErrorTracking == nil {
		a.ErrorTracking = &ErrorTracking{}
	}
	a.ErrorTracking.RateLimitCount++
	a.ErrorTracking.LastError = "HTTP 429: Rate Limit Exceeded"
	now := time.Now().Unix()
	a.ErrorTracking.LastErrorTime = &now

	// Adaptive backoff: start at 120s (2min), double each time, max 30 minutes
	backoffSeconds := int64(120)
	if a.ErrorTracking.RateLimitBackoff > 0 {
		backoffSeconds = a.ErrorTracking.RateLimitBackoff * 2
	}
	if backoffSeconds > 1800 {
		backoffSeconds = 1800 // Max 30 minutes
	}
	a.ErrorTracking.RateLimitBackoff = backoffSeconds
	failedUntil := now + backoffSeconds
	a.ErrorTracking.FailedUntil = &failedUntil
}

// RecordPermissionDenied handles 403 permission denied errors
func (a *Account) RecordPermissionDenied() {
	a.RefreshStatus = "permission_denied"
	a.Enable = false // Disable account immediately
	if a.ErrorTracking == nil {
		a.ErrorTracking = &ErrorTracking{}
	}
	a.ErrorTracking.IsPermissionDenied = true
	a.ErrorTracking.LastError = "HTTP 403: Permission Denied - Account does not have required entitlements"
	now := time.Now().Unix()
	a.ErrorTracking.LastErrorTime = &now
}

// RecordUsage updates usage statistics
func (a *Account) RecordUsage(inputTokens, outputTokens int64) {
	if a.Usage == nil {
		a.Usage = &UsageStats{}
	}
	a.Usage.RequestCount++
	a.Usage.TotalTokens += inputTokens + outputTokens
	a.Usage.InputTokens += inputTokens
	a.Usage.OutputTokens += outputTokens
	now := time.Now().UnixMilli()
	a.Usage.LastUsed = &now
}
