// Package alibaba — auth constants and token storage types.
package alibaba

import (
	"net/http"
	"time"
)

var (
	alibabaHTTPClient   = &http.Client{Timeout: 120 * time.Second}
	alibabaStreamClient = &http.Client{}
)

// ─── Base URL constants ───────────────────────────────────────────────────────

const (
	// BaseURLGlobal is the DashScope endpoint for the international region.
	BaseURLGlobal = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	// BaseURLChina is the DashScope endpoint for mainland China.
	BaseURLChina = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	// CodingPlanBaseURLGlobal is the Coding Plan endpoint for the international region.
	CodingPlanBaseURLGlobal = "https://coding-intl.dashscope.aliyuncs.com/v1"
	// CodingPlanBaseURLChina is the Coding Plan endpoint for mainland China.
	CodingPlanBaseURLChina = "https://coding.dashscope.aliyuncs.com/v1"
)

// ─── TokenData ────────────────────────────────────────────────────────────────

// TokenData is the persisted credential record for an Alibaba instance.
type TokenData struct {
	AuthType    string `json:"auth_type"`          // always "api-key"
	AccessToken string `json:"access_token"`       // the DashScope API key
	BaseURL     string `json:"base_url,omitempty"` // optional explicit base URL
}
