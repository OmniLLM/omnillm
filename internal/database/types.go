// Package database provides SQLite-based persistence for OmniLLM
package database

import "time"

// parseTime converts a string timestamp to time.Time
// SQLite returns timestamps as strings from modernc.org/sqlite
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try RFC3339 format first (ISO 8601 with timezone)
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	// Try RFC3339Nano format
	t, err = time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t
	}
	// Try SQLite datetime format
	t, err = time.Parse("2006-01-02 15:04:05", s)
	if err == nil {
		return t
	}
	// Return zero time if parsing fails
	return time.Time{}
}

type ConfigRecord struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProviderInstanceRecord struct {
	InstanceID string    `json:"instance_id"`
	ProviderID string    `json:"provider_id"`
	Name       string    `json:"name"`
	Priority   int       `json:"priority"`
	Activated  bool      `json:"activated"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type TokenRecord struct {
	InstanceID string    `json:"instance_id"`
	ProviderID string    `json:"provider_id"`
	TokenData  string    `json:"token_data"` // JSON string
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ProviderModelStateRecord struct {
	InstanceID string    `json:"instance_id"`
	ModelID    string    `json:"model_id"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ProviderConfigRecord struct {
	InstanceID string    `json:"instance_id"`
	ConfigData string    `json:"config_data"` // JSON string
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ModelConfigRecord struct {
	InstanceID string    `json:"instance_id"`
	ModelID    string    `json:"model_id"`
	Version    string    `json:"version"`
	ConfigData string    `json:"config_data"` // JSON string - for model-specific configuration
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ProviderModelsCacheRecord struct {
	InstanceID string    `json:"instance_id"`
	ModelsData string    `json:"models_data"` // JSON array of cached models
	CachedAt   time.Time `json:"cached_at"`
}

type ChatSessionRecord struct {
	SessionID string    `json:"session_id"`
	Title     string    `json:"title"`
	ModelID   string    `json:"model_id"`
	APIShape  string    `json:"api_shape"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ChatMessageRecord struct {
	MessageID string    `json:"message_id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"` // user, assistant, system
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type LbStrategy string

const (
	LbStrategyRoundRobin LbStrategy = "round-robin"
	LbStrategyRandom     LbStrategy = "random"
	LbStrategyPriority   LbStrategy = "priority"
	LbStrategyWeighted   LbStrategy = "weighted"
)

type VirtualModelRecord struct {
	VirtualModelID string     `json:"virtual_model_id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	APIShape       string     `json:"api_shape"`
	LbStrategy     LbStrategy `json:"lb_strategy"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type VirtualModelUpstreamRecord struct {
	ID             int64     `json:"id"`
	VirtualModelID string    `json:"virtual_model_id"`
	ProviderID     string    `json:"provider_id"`
	ModelID        string    `json:"model_id"`
	Weight         int       `json:"weight"`
	Priority       int       `json:"priority"`
	CreatedAt      time.Time `json:"created_at"`
}
