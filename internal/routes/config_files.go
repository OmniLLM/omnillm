package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// configFilePaths maps config names to their absolute file paths on the user's machine.
var configFilePaths = map[string]string{
	"claude-code": expandHomePath("~/.claude/settings.json"),
	"codex":       expandHomePath("~/.codex/config.toml"),
	"droid":       expandHomePath("~/.factory/settings.json"),
	"opencode":    expandHomePath("~/.opencode/config.json"),
	"amp":         expandHomePath("~/.amp/config.json"),
}

// ConfigFilePathsForTest returns a shallow copy of the current config file path map.
func ConfigFilePathsForTest() map[string]string {
	copyMap := make(map[string]string, len(configFilePaths))
	for k, v := range configFilePaths {
		copyMap[k] = v
	}
	return copyMap
}

// SetConfigFilePathsForTest replaces config file paths for tests.
func SetConfigFilePathsForTest(paths map[string]string) {
	configFilePaths = paths
}

// configMetadata holds display info for each config file.
var configMetadata = map[string]struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Language    string `json:"language"`
}{
	"claude-code": {
		Label:       "Claude Code Settings",
		Description: "Claude Code configuration file (~/.claude/settings.json)",
		Language:    "json",
	},
	"codex": {
		Label:       "Codex Configuration",
		Description: "OpenAI Codex configuration file (~/.codex/config.toml)",
		Language:    "toml",
	},
	"droid": {
		Label:       "Droid Configuration",
		Description: "Droid AI CLI configuration file (~/.factory/settings.json)",
		Language:    "json",
	},
	"opencode": {
		Label:       "OpenCode Settings",
		Description: "OpenCode CLI configuration file (~/.opencode/config.json)",
		Language:    "json",
	},
	"amp": {
		Label:       "Amp Configuration",
		Description: "Amp AI CLI configuration file (~/.amp/config.json)",
		Language:    "json",
	},
}

// expandHomePath replaces ~ with the user's home directory.
func expandHomePath(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// handleGetConfigFiles returns the list of available config files.
func handleGetConfigFiles(c *gin.Context) {
	type ConfigFile struct {
		Name        string `json:"name"`
		Label       string `json:"label"`
		Description string `json:"description"`
		Language    string `json:"language"`
		Exists      bool   `json:"exists"`
	}

	result := make([]ConfigFile, 0, len(configFilePaths))
	for name, path := range configFilePaths {
		meta := configMetadata[name]
		exists := false
		if _, err := os.Stat(path); err == nil {
			exists = true
		}
		result = append(result, ConfigFile{
			Name:        name,
			Label:       meta.Label,
			Description: meta.Description,
			Language:    meta.Language,
			Exists:      exists,
		})
	}

	c.JSON(http.StatusOK, gin.H{"configs": result})
}

// handleGetConfig reads a config file and returns its content.
func handleGetConfig(c *gin.Context) {
	name := c.Param("name")

	filePath, ok := configFilePaths[name]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Unknown config: %s", name)})
		return
	}

	meta := configMetadata[name]

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{
				"name":    name,
				"label":   meta.Label,
				"content": "",
				"exists":  false,
				"message": "File does not exist yet",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read file: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"name":    name,
		"label":   meta.Label,
		"content": string(data),
		"exists":  true,
	})
}

// handleSaveConfig saves the provided content to a config file.
func handleSaveConfig(c *gin.Context) {
	if !getSecurityOptions().EnableConfigEdit {
		c.JSON(http.StatusForbidden, gin.H{"error": "config editing is disabled"})
		return
	}

	name := c.Param("name")

	filePath, ok := configFilePaths[name]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Unknown config: %s", name)})
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// For JSON files, validate before saving
	if name == "claude-code" || name == "opencode" || name == "droid" || name == "amp" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(req.Content), &js); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid JSON: %v", err)})
			return
		}
	}

	// Ensure parent directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create directory: %v", err)})
		return
	}

	if err := os.WriteFile(filePath, []byte(req.Content), 0o600); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to write file: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Configuration saved to %s", filePath),
	})
}

// handleImportConfig accepts a file upload and saves its content to the config path.
func handleImportConfig(c *gin.Context) {
	if !getSecurityOptions().EnableConfigEdit {
		c.JSON(http.StatusForbidden, gin.H{"error": "config editing is disabled"})
		return
	}

	name := c.Param("name")

	filePath, ok := configFilePaths[name]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Unknown config: %s", name)})
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read uploaded file: %v", err)})
		return
	}

	// For JSON files, validate before saving
	if name == "claude-code" || name == "opencode" || name == "droid" || name == "amp" {
		var js json.RawMessage
		if err := json.Unmarshal(content, &js); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid JSON in uploaded file: %v", err)})
			return
		}
	}

	// Ensure parent directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create directory: %v", err)})
		return
	}

	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to write file: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Configuration imported to %s", filePath),
	})
}

// handleBackupConfig creates a timestamped copy of the config file in the same directory.
func handleBackupConfig(c *gin.Context) {
	name := c.Param("name")

	filePath, ok := configFilePaths[name]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Unknown config: %s", name)})
		return
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Config file does not exist yet"})
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read file: %v", err)})
		return
	}

	ext := filepath.Ext(filePath)
	base := strings.TrimSuffix(filePath, ext)
	timestamp := time.Now().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.%s%s", base, timestamp, ext)

	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create backup: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"backup":  backupPath,
		"message": fmt.Sprintf("Backup saved to %s", backupPath),
	})
}
