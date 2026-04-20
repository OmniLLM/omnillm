package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	apiKeyFileName = "api-key"
	apiKeyEnvVar   = "OMNILLM_API_KEY"
)

func resolveAPIKey(configDir, explicit string) (string, error) {
	if key := strings.TrimSpace(explicit); key != "" {
		return key, nil
	}

	if key := strings.TrimSpace(os.Getenv(apiKeyEnvVar)); key != "" {
		return key, nil
	}

	path := filepath.Join(configDir, apiKeyFileName)
	if data, err := os.ReadFile(path); err == nil {
		if key := strings.TrimSpace(string(data)); key != "" {
			return key, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read api key: %w", err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	key := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		return "", fmt.Errorf("persist api key: %w", err)
	}
	return key, nil
}
