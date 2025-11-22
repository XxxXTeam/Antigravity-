package models

import (
	"time"
)

// APIKey represents an API access key
type APIKey struct {
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	RateLimit  *RateLimit `json:"rateLimit,omitempty"`
	CreatedAt  int64      `json:"createdAt"`
	LastUsed   *int64     `json:"lastUsed,omitempty"`
	UsageCount int64      `json:"usageCount"`
}

// RateLimit defines rate limiting for an API key
type RateLimit struct {
	Enabled     bool `json:"enabled"`
	MaxRequests int  `json:"maxRequests"`
	WindowMs    int  `json:"windowMs"`
}

// IsRateLimited checks if key is currently rate limited
func (k *APIKey) IsRateLimited(requests int, window time.Duration) bool {
	if k.RateLimit == nil || !k.RateLimit.Enabled {
		return false
	}
	return requests >= k.RateLimit.MaxRequests
}

// UpdateUsage updates the key's usage statistics
func (k *APIKey) UpdateUsage() {
	now := time.Now().Unix()
	k.LastUsed = &now
	k.UsageCount++
}
