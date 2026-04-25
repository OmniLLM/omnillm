package generic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"omnillm/internal/database"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"

	alibabapkg "omnillm/internal/providers/alibaba"
	azurepkg "omnillm/internal/providers/azure"
	googlepkg "omnillm/internal/providers/google"
	kimipkg "omnillm/internal/providers/kimi"

	"github.com/rs/zerolog/log"
)

// SetupAuth authenticates the provider using the given options.
func (p *GenericProvider) SetupAuth(options *types.AuthOptions) error {
	switch p.id {
	case "alibaba":
		return p.setupAlibabaAuth(options)
	case "antigravity":
		return p.setupAntigravityAuth(options)
	case "azure-openai":
		return p.setupAzureAuth(options)
	case "google":
		return p.setupGoogleAuth(options)
	case "kimi":
		return p.setupKimiAuth(options)
	default:
		return fmt.Errorf("use the admin UI to authenticate %s", p.id)
	}
}

func (p *GenericProvider) setupAlibabaAuth(options *types.AuthOptions) error {
	switch options.Method {
	case "api-key", "":
		token, baseURL, name, config, err := alibabapkg.SetupAPIKeyAuth(p.instanceID, options)
		if err != nil {
			return err
		}
		p.token = token
		p.baseURL = baseURL
		p.name = name
		p.config = config
		return nil

	default:
		return fmt.Errorf("alibaba: unsupported auth method: %s (only api-key is supported)", options.Method)
	}
}

// setupAntigravityAuth stores Google OAuth client credentials and initiates a
// Google device-code flow. The access token is obtained asynchronously; the
// route handler is responsible for polling and completing the flow.
func (p *GenericProvider) setupAntigravityAuth(options *types.AuthOptions) error {
	clientID := strings.TrimSpace(options.ClientID)
	clientSecret := strings.TrimSpace(options.ClientSecret)
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("antigravity: OAuth client_id and client_secret are required")
	}

	// Persist the credentials so the device-code poll goroutine can retrieve them.
	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{
		"client_id":     clientID,
		"client_secret": clientSecret,
	}
	if err := tokenStore.Save(p.instanceID, "antigravity", tokenData); err != nil {
		return fmt.Errorf("antigravity: failed to save client credentials: %w", err)
	}

	p.baseURL = providerBaseURLs["antigravity"]
	suffix := shared.ShortTokenSuffix(clientID)
	p.name = "Antigravity (" + suffix + ")"

	log.Info().Str("provider", p.instanceID).Msg("Antigravity: client credentials saved, device-code flow will be initiated")
	return nil
}

// InitiateGoogleDeviceCode starts Google's OAuth2 device-code flow for Antigravity.
// Returns the verification URL and user code so the frontend can show them to the user.
func (p *GenericProvider) InitiateGoogleDeviceCode() (verificationURL, userCode, deviceCode string, interval int, err error) {
	// Load credentials from DB.
	tokenStore := database.NewTokenStore()
	record, dbErr := tokenStore.Get(p.instanceID)
	if dbErr != nil || record == nil {
		return "", "", "", 0, fmt.Errorf("antigravity: client credentials not found — set them first")
	}
	var saved map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(record.TokenData), &saved); jsonErr != nil {
		return "", "", "", 0, fmt.Errorf("antigravity: failed to read saved credentials: %w", jsonErr)
	}
	clientID, _ := saved["client_id"].(string)
	clientSecret, _ := saved["client_secret"].(string)
	if clientID == "" || clientSecret == "" {
		return "", "", "", 0, fmt.Errorf("antigravity: client_id/client_secret not set")
	}

	// Google device authorization endpoint.
	log.Info().
		Str("provider", p.instanceID).
		Bool("has_client_id", clientID != "").
		Bool("has_client_secret", clientSecret != "").
		Msg("Antigravity: requesting Google device code")
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("scope", "https://www.googleapis.com/auth/cloud-platform openid email")

	req, reqErr := http.NewRequest("POST", "https://oauth2.googleapis.com/device/code",
		bytes.NewBufferString(data.Encode()))
	if reqErr != nil {
		return "", "", "", 0, fmt.Errorf("antigravity: failed to build device-code request: %w", reqErr)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, respErr := client.Do(req)
	if respErr != nil {
		return "", "", "", 0, fmt.Errorf("antigravity: device-code request failed: %w", respErr)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", "", "", 0, fmt.Errorf("antigravity: device-code endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var dc struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_url"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if jsonErr := json.Unmarshal(body, &dc); jsonErr != nil {
		return "", "", "", 0, fmt.Errorf("antigravity: failed to parse device-code response: %w", jsonErr)
	}
	verificationURL = dc.VerificationURL
	if verificationURL == "" {
		verificationURL = dc.VerificationURI
	}
	if dc.DeviceCode == "" || dc.UserCode == "" || verificationURL == "" {
		return "", "", "", 0, fmt.Errorf("antigravity: incomplete device-code response: %s", string(body))
	}

	log.Info().Str("provider", p.instanceID).Str("user_code", dc.UserCode).Msg("Antigravity: Google device-code flow initiated")
	interval = dc.Interval
	if interval == 0 {
		interval = 5
	}
	return verificationURL, dc.UserCode, dc.DeviceCode, interval, nil
}

// PollGoogleDeviceCode polls Google's token endpoint until the user completes
// authorization, then stores the access and refresh tokens.
func (p *GenericProvider) PollGoogleDeviceCode(deviceCode string) error {
	tokenStore := database.NewTokenStore()
	record, dbErr := tokenStore.Get(p.instanceID)
	if dbErr != nil || record == nil {
		return fmt.Errorf("antigravity: client credentials not found")
	}
	var saved map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(record.TokenData), &saved); jsonErr != nil {
		return fmt.Errorf("antigravity: failed to read saved credentials: %w", jsonErr)
	}
	clientID, _ := saved["client_id"].(string)
	clientSecret, _ := saved["client_secret"].(string)

	tokenURL := "https://oauth2.googleapis.com/token"
	for {
		data := url.Values{}
		data.Set("client_id", clientID)
		data.Set("client_secret", clientSecret)
		data.Set("device_code", deviceCode)
		data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

		req, _ := http.NewRequest("POST", tokenURL, bytes.NewBufferString(data.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, respErr := client.Do(req)
		if respErr != nil {
			return fmt.Errorf("antigravity: token poll failed: %w", respErr)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var tokenResp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int64  `json:"expires_in"`
			Error        string `json:"error"`
		}
		if jsonErr := json.Unmarshal(body, &tokenResp); jsonErr != nil {
			return fmt.Errorf("antigravity: failed to parse token response: %w", jsonErr)
		}

		switch tokenResp.Error {
		case "":
			if tokenResp.AccessToken == "" {
				return fmt.Errorf("antigravity: no access_token in response")
			}
			p.token = tokenResp.AccessToken
			// Persist the final tokens alongside the credentials.
			saved["access_token"] = tokenResp.AccessToken
			if tokenResp.RefreshToken != "" {
				saved["refresh_token"] = tokenResp.RefreshToken
			}
			if err := tokenStore.Save(p.instanceID, "antigravity", saved); err != nil {
				return fmt.Errorf("antigravity: failed to save token: %w", err)
			}
			log.Info().Str("provider", p.instanceID).Msg("Antigravity: Google OAuth completed")
			return nil
		case "authorization_pending", "slow_down":
			time.Sleep(5 * time.Second)
			continue
		case "expired_token":
			return fmt.Errorf("antigravity: device code expired — please restart authentication")
		case "access_denied":
			return fmt.Errorf("antigravity: access denied by user")
		default:
			return fmt.Errorf("antigravity: token error: %s — %s", tokenResp.Error, string(body))
		}
	}
}

// loadAlibabaTokenFromDB reads the persisted Alibaba token and applies it to the provider.
func (p *GenericProvider) loadAlibabaTokenFromDB() error {
	token, baseURL, config, err := alibabapkg.LoadTokenFromDB(p.instanceID)
	if err != nil {
		return err
	}
	if token != "" {
		p.token = token
	}
	if baseURL != "" {
		p.baseURL = baseURL
	}
	if config != nil {
		p.applyConfig(config)
	}
	return nil
}

// SaveAlibabaOAuthToken is a stub that returns an error since OAuth is no longer supported
func (p *GenericProvider) SaveAlibabaOAuthToken(td *alibabapkg.TokenData) (newInstanceID string, err error) {
	return "", fmt.Errorf("oauth authentication is no longer supported for Alibaba DashScope - please use API key authentication")
}

func (p *GenericProvider) setupAzureAuth(options *types.AuthOptions) error {
	token, endpoint, cfg, err := azurepkg.SetupAuth(p.instanceID, options)
	if err != nil {
		return err
	}
	p.token = token
	if endpoint != "" {
		p.baseURL = endpoint
		if name := deriveAzureName(endpoint); name != "" {
			p.name = name
		}
	}
	if cfg != nil {
		p.applyConfig(cfg)
	}
	return nil
}

func (p *GenericProvider) setupGoogleAuth(options *types.AuthOptions) error {
	token, baseURL, name, err := googlepkg.SetupAuth(p.instanceID, options)
	if err != nil {
		return err
	}
	p.token = token
	p.baseURL = baseURL
	p.name = name
	return nil
}

func (p *GenericProvider) setupKimiAuth(options *types.AuthOptions) error {
	switch options.Method {
	case "", "api-key":
		token, baseURL, name, config, err := kimipkg.SetupAPIKeyAuth(p.instanceID, options)
		if err != nil {
			return err
		}
		p.token = token
		p.baseURL = baseURL
		p.name = name
		p.config = config
		return nil

	case "oauth":
		return p.loadKimiTokenFromDB()

	default:
		return fmt.Errorf("kimi: unsupported auth method: %s", options.Method)
	}
}

// loadKimiTokenFromDB reads the persisted Kimi token and applies it to the provider.
func (p *GenericProvider) loadKimiTokenFromDB() error {
	token, baseURL, config, err := kimipkg.LoadTokenFromDB(p.instanceID)
	if err != nil {
		return err
	}
	if token != "" {
		p.token = token
	}
	if baseURL != "" {
		p.baseURL = baseURL
	}
	if config != nil {
		p.applyConfig(config)
	}
	return nil
}

// SaveKimiOAuthToken persists a completed OAuth token and returns the new canonical instance ID.
func (p *GenericProvider) SaveKimiOAuthToken(td *kimipkg.TokenData) (newInstanceID string, err error) {
	newID, name, baseURL, err := kimipkg.SaveOAuthToken(p.instanceID, td)
	if err != nil {
		return "", err
	}
	p.token = td.AccessToken
	p.name = name
	p.baseURL = baseURL
	p.instanceID = newID
	p.applyConfig(map[string]interface{}{
		"auth_type":    td.AuthType,
		"base_url":     td.BaseURL,
		"resource_url": td.ResourceURL,
	})
	return newID, nil
}
