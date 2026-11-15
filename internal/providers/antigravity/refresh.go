package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"omnillm/internal/database"
	"time"
)

// credentials serializes renewal, then returns a private snapshot for the request.
// A rejected token forces renewal unless another request already replaced it.
func (p *Provider) credentials(ctx context.Context, rejected string) (map[string]interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case p.refreshGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-p.refreshGate }()
	store := database.NewTokenStore()
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record, err := store.Get(p.instanceID)
		if err != nil {
			return nil, fmt.Errorf("antigravity: load credentials: %w", err)
		}
		if record == nil {
			return nil, fmt.Errorf("antigravity: credentials missing; re-authenticate this provider")
		}
		d := map[string]interface{}{}
		if err := json.Unmarshal([]byte(record.TokenData), &d); err != nil {
			d = map[string]interface{}{"access_token": record.TokenData}
		}
		if d == nil {
			return nil, fmt.Errorf("antigravity: invalid credentials; re-authenticate this provider")
		}
		token, _ := d["access_token"].(string)
		refresh, _ := d["refresh_token"].(string)
		client, _ := d["client_id"].(string)
		secret, _ := d["client_secret"].(string)
		expiry, _ := d["expires_at"].(float64)
		forced := rejected == "*" || (rejected != "" && rejected == token)
		due := expiry > 0 && expiry <= float64(time.Now().Unix()+60)
		legacy := expiry == 0 && refresh != ""
		if !forced && !due && !legacy && token != "" {
			return d, nil
		}
		if refresh == "" || client == "" || secret == "" {
			return nil, fmt.Errorf("antigravity: refresh credentials missing; re-authenticate this provider")
		}
		renewed, err := refreshAccessToken(ctx, client, secret, refresh)
		if err != nil {
			return nil, err
		}
		d["access_token"] = renewed.AccessToken
		if renewed.RefreshToken != "" {
			d["refresh_token"] = renewed.RefreshToken
		}
		delete(d, "expires_at")
		if renewed.ExpiresIn > 0 {
			d["expires_at"] = time.Now().Unix() + renewed.ExpiresIn
		}
		saved, err := store.SaveIfUnchanged(ctx, p.instanceID, record.TokenData, d)
		if err != nil {
			return nil, fmt.Errorf("antigravity: save refreshed credentials: %w", err)
		}
		if !saved {
			continue
		}
		p.mu.Lock()
		p.token = renewed.AccessToken
		p.mu.Unlock()
		return d, nil
	}
	return nil, fmt.Errorf("antigravity: credentials changed repeatedly; retry request")
}
