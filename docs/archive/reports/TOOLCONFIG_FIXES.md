# ToolConfig Path Fixes

## Issues Fixed

### 1. Wrong File Paths Displayed in UI ✅

The ToolConfig UI was showing incorrect file paths for Droid, OpenCode, and AMP configurations.

**Problem:**
- All three tools showed generic paths like `~/.codex/config.toml` or `~/.claude/settings.json`
- Backend API returned wrong paths in `configFilePaths` map
- Frontend `ToolCard` component hardcoded descriptions

**Root Cause:**
- Backend (`internal/routes/config_files.go`) had incorrect paths:
  - Droid: `~/.droid/config.toml` ❌ → Should be `~/.factory/settings.json` ✅
  - OpenCode: `~/.opencode/settings.json` ❌ → Should be `~/.opencode/config.json` ✅
  - AMP: `~/.amp/config.toml` ❌ → Should be `~/.amp/config.json` ✅

- Backend metadata had wrong descriptions and languages:
  - Droid: TOML ❌ → JSON ✅
  - AMP: TOML ❌ → JSON ✅

- Frontend `ToolCard` component (line 1010-1013) hardcoded:
  ```typescript
  const desc = entry.language === "json" ? "~/.claude/settings.json" : "~/.codex/config.toml"
  ```

### 2. Save Doesn't Reload Config Immediately ✅

After clicking Save, the UI didn't refresh to show updated file status or content.

**Problem:**
- Save operation only updated local state
- Didn't reload config list to update "exists" badge
- Didn't re-fetch current config to ensure latest data

## Changes Made

### Backend Changes (`internal/routes/config_files.go`)

#### 1. Fixed File Paths (Lines 16-22)
```go
var configFilePaths = map[string]string{
	"claude-code": expandHomePath("~/.claude/settings.json"),
	"codex":       expandHomePath("~/.codex/config.toml"),
	"droid":       expandHomePath("~/.factory/settings.json"),    // ✅ Fixed
	"opencode":    expandHomePath("~/.opencode/config.json"),     // ✅ Fixed
	"amp":         expandHomePath("~/.amp/config.json"),          // ✅ Fixed
}
```

#### 2. Fixed Metadata (Lines 40-54)
```go
"droid": {
	Label:       "Droid Configuration",
	Description: "Droid AI CLI configuration file (~/.factory/settings.json)", // ✅ Fixed
	Language:    "json",                                                        // ✅ Fixed
},
"opencode": {
	Label:       "OpenCode Settings",
	Description: "OpenCode CLI configuration file (~/.opencode/config.json)",  // ✅ Fixed
	Language:    "json",
},
"amp": {
	Label:       "Amp Configuration",
	Description: "Amp AI CLI configuration file (~/.amp/config.json)",         // ✅ Fixed
	Language:    "json",                                                        // ✅ Fixed
},
```

#### 3. Updated JSON Validation (Lines 158, 213)
Added droid and amp to JSON validation:
```go
// For JSON files, validate before saving
if name == "claude-code" || name == "opencode" || name == "droid" || name == "amp" {
	var js json.RawMessage
	if err := json.Unmarshal([]byte(req.Content), &js); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid JSON: %v", err)})
		return
	}
}
```

### Frontend Changes (`frontend/src/pages/ConfigPage.tsx`)

#### 1. Fixed ToolCard Descriptions (Lines 1000-1027)
```typescript
function ToolCard({
  entry,
  isActive,
  onClick,
}: {
  entry: ConfigFileEntry
  isActive: boolean
  onClick: () => void
}) {
  const Icon = entry.language === "json" ? FileJson : FileText

  // Map config names to their actual file paths
  const configPaths: Record<string, string> = {
    "claude-code": "~/.claude/settings.json",
    "codex": "~/.codex/config.toml",
    "droid": "~/.factory/settings.json",      // ✅ Correct
    "opencode": "~/.opencode/config.json",    // ✅ Correct
    "amp": "~/.amp/config.json",              // ✅ Correct
  }

  const desc = configPaths[entry.name] || (
    entry.language === "json" ? "~/.config/settings.json" : "~/.config/config.toml"
  )
```

#### 2. Enhanced Save to Reload Config (Lines 1179-1223)
```typescript
const handleSave = () => {
  if (!activeConfig) return
  const content = getContentToSave()
  setSaving(true)
  saveConfigFile(activeConfig, content)
    .then(() => {
      setOriginalContent(content)
      setDirty(false)
      showToast("Configuration saved", "success")

      // ✅ Reload the config list to update the "exists" status
      return listConfigFiles()
    })
    .then((r) => {
      setConfigs(r.configs)

      // ✅ Reload the current config to ensure we have the latest data
      if (activeConfig) {
        return getConfigFile(activeConfig)
      }
    })
    .then((resp) => {
      if (resp) {
        setRawContent(resp.content)
        setOriginalContent(resp.content)

        // ✅ Re-parse structured data
        if (activeConfig === "claude-code" && resp.content) {
          try {
            setClaudeSettings(JSON.parse(resp.content))
          } catch {
            setClaudeSettings(null)
          }
        } else if (activeConfig === "codex" && resp.content) {
          try {
            setCodexConfig(parseTOML(resp.content))
          } catch {
            setCodexConfig(null)
          }
        }
      }
    })
    .catch((err: Error) => showToast(`Save failed: ${err.message}`, "error"))
    .finally(() => setSaving(false))
}
```

## Testing Checklist

- [x] Backend file paths corrected
- [x] Backend metadata updated (descriptions and languages)
- [x] JSON validation includes all JSON configs
- [x] Frontend ToolCard shows correct paths
- [x] Save operation reloads config list
- [x] Save operation reloads current config content
- [x] Save operation updates structured editors
- [x] Frontend builds successfully

## Expected Behavior After Fix

### UI Shows Correct Paths

When you open ToolConfig, you should now see:

| Tool | Label | Path Shown | Language |
|------|-------|------------|----------|
| Claude Code | Claude Code Settings | `~/.claude/settings.json` | JSON |
| Codex | Codex Configuration | `~/.codex/config.toml` | TOML |
| **Droid** | Droid Configuration | **`~/.factory/settings.json`** | **JSON** ✅ |
| **OpenCode** | OpenCode Settings | **`~/.opencode/config.json`** | **JSON** ✅ |
| **AMP** | Amp Configuration | **`~/.amp/config.json`** | **JSON** ✅ |

### Save Reloads Immediately

After editing and clicking Save:
1. ✅ Config file is written to disk
2. ✅ Toast notification shows "Configuration saved"
3. ✅ Config list is reloaded (updates "exists" badges)
4. ✅ Current config is re-fetched (ensures latest data)
5. ✅ Structured editor is updated with new content
6. ✅ "Unsaved changes" indicator disappears

## Files Modified

| File | Changes |
|------|---------|
| `internal/routes/config_files.go` | Fixed paths, metadata, and validation for droid/opencode/amp |
| `frontend/src/pages/ConfigPage.tsx` | Fixed ToolCard descriptions and enhanced save to reload |

## Next Steps

1. Restart OmniLLM backend to apply backend changes:
   ```bash
   bun run omni restart --rebuild
   ```

2. Refresh browser to load updated frontend

3. Verify each config shows correct path in ToolConfig UI

4. Test saving a config and confirm it reloads immediately
