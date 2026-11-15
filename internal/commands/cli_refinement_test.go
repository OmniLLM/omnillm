package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"omnillm/internal/database"
	"omnillm/internal/server"
)

func cloneCommandTree(src *cobra.Command) *cobra.Command {
	dst := &cobra.Command{
		Use:                   src.Use,
		Aliases:               append([]string(nil), src.Aliases...),
		Short:                 src.Short,
		Long:                  src.Long,
		Example:               src.Example,
		Args:                  src.Args,
		ValidArgs:             append([]string(nil), src.ValidArgs...),
		ValidArgsFunction:     src.ValidArgsFunction,
		RunE:                  src.RunE,
		Hidden:                src.Hidden,
		Deprecated:            src.Deprecated,
		DisableFlagParsing:    src.DisableFlagParsing,
		DisableFlagsInUseLine: src.DisableFlagsInUseLine,
		TraverseChildren:      src.TraverseChildren,
		SilenceUsage:          src.SilenceUsage,
		SilenceErrors:         src.SilenceErrors,
	}
	dst.Flags().AddFlagSet(src.LocalFlags())
	for _, sub := range src.Commands() {
		dst.AddCommand(cloneCommandTree(sub))
	}
	return dst
}

func buildRefinementTestRoot(t *testing.T, serverURL, output string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	t.Setenv("OMNILLM_SERVER", serverURL)
	t.Setenv("OMNILLM_API_KEY", "test-key")

	root := &cobra.Command{Use: "omnillm", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("server", serverURL, "")
	root.PersistentFlags().String("api-key", "test-key", "")
	root.PersistentFlags().StringP("output", "o", output, "")

	for _, cmd := range []*cobra.Command{
		StartCmd,
		RestartCmd,
		ProviderCmd,
		VirtualModelCmd,
		ConfigCmd,
		SettingsCmd,
		StatusCmd,
		LogsCmd,
		UsageCmd,
		CheckUsageCmd,
		DoctorCmd,
		DebugCmd,
		SyncNamesCmd,
		CompletionCmd,
	} {
		root.AddCommand(cloneCommandTree(cmd))
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	return root, &out
}

func TestStartAndRestartRemoveDeadNoOpFlags(t *testing.T) {
	deadFlags := []string{"account-type", "proxy-env", "provider", "console"}
	for _, cmd := range []*cobra.Command{StartCmd, RestartCmd} {
		for _, flag := range deadFlags {
			if cmd.Flags().Lookup(flag) != nil {
				t.Errorf("%s still exposes dead flag --%s", cmd.Name(), flag)
			}
		}
		if cmd.Flags().Lookup("allow-local-endpoints") == nil {
			t.Errorf("%s missing --allow-local-endpoints", cmd.Name())
		}
	}
	if providerLoginCmd.Flags().Lookup("allow-local-endpoints") == nil {
		t.Error("provider login missing --allow-local-endpoints")
	}
}

func TestStartClaudeCodeOutputsReachableGuidanceWithoutDummyKey(t *testing.T) {
	requireShellBackgroundTest(t)
	script := writeBackgroundHelper(t, `printf '{"ready":true,"pid":%d,"address":"http://0.0.0.0:5000"}' "$$" > "$OMNILLM_INTERNAL_READINESS_PATH"
exec sleep 30`)
	restoreBackgroundGlobals(t, script, time.Second)

	var out bytes.Buffer
	cmd := &cobra.Command{Use: "start"}
	addStartFlags(cmd)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Flags().Set("host", "0.0.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("claude-code", "true"); err != nil {
		t.Fatal(err)
	}
	if err := StartCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("start with --claude-code: %v", err)
	}

	t.Cleanup(func() {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		for _, line := range lines {
			if !strings.Contains(line, "PID ") {
				continue
			}
			start := strings.Index(line, "PID ") + 4
			rest := line[start:]
			if end := strings.Index(rest, ","); end > 0 {
				if pid, err := strconv.Atoi(strings.TrimSpace(rest[:end])); err == nil && pid > 0 {
					if proc, err := os.FindProcess(pid); err == nil {
						_ = proc.Kill()
					}
				}
			}
		}
	})

	text := out.String()
	if !strings.Contains(text, "# Claude Code Configuration") {
		t.Fatalf("missing # Claude Code Configuration section:\n%s", text)
	}
	guidance := strings.SplitN(text, "# Claude Code Configuration", 2)[1]
	if !strings.Contains(guidance, "http://127.0.0.1:5000/v1") {
		t.Fatalf("expected loopback client URL in output:\n%s", text)
	}
	if strings.Contains(guidance, "0.0.0.0") {
		t.Fatalf("client URL should not advertise 0.0.0.0:\n%s", text)
	}
	if strings.Contains(guidance, "dummy") {
		t.Fatalf("claude-code output must not emit dummy API key:\n%s", text)
	}
	if !strings.Contains(guidance, "ANTHROPIC_BASE_URL") {
		t.Fatalf("missing ANTHROPIC_BASE_URL guidance:\n%s", text)
	}
	_ = server.StartOptions{}
}

func TestUsageValidatesTimestampsAndAPIShapeHelp(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if f := UsageCmd.Flags().Lookup("api-shape"); f == nil || !strings.Contains(f.Usage, "responses") {
		t.Fatalf("usage --api-shape help should mention responses: %v", f)
	}

	for _, flag := range []string{"since", "until"} {
		t.Run("invalid-"+flag, func(t *testing.T) {
			root, _ := buildRefinementTestRoot(t, srv.URL, "table")
			t.Cleanup(func() { _ = UsageCmd.Flags().Set(flag, "") })
			root.SetArgs([]string{"usage", "--" + flag, "2026-11-15"})
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "RFC3339") {
				t.Fatalf("expected RFC3339 validation error for --%s, got %v", flag, err)
			}
			if requests.Load() != 0 {
				t.Fatalf("invalid timestamp sent %d server requests", requests.Load())
			}
		})
	}
}

func TestSettingsResponseCacheRejectsNegativeTTL(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	root, _ := buildRefinementTestRoot(t, srv.URL, "table")
	t.Cleanup(func() {
		_ = settingsSetResponseCacheCmd.Flags().Set("ttl", "-1")
	})
	root.SetArgs([]string{"settings", "set", "response-cache", "on", "--ttl", "-5"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--ttl") {
		t.Fatalf("expected negative --ttl validation error, got %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("negative ttl sent %d server requests", requests.Load())
	}
}

func TestZeroArgCommandsRejectExtraArgs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	commands := [][]string{
		{"provider", "list", "extra"},
		{"provider", "priorities", "extra"},
		{"virtualmodel", "list", "extra"},
		{"config", "list", "extra"},
		{"settings", "get", "log-level", "extra"},
		{"settings", "get", "response-cache", "extra"},
		{"settings", "clear-response-cache", "extra"},
		{"status", "extra"},
		{"status", "auth", "extra"},
		{"usage", "extra"},
		{"check-usage", "extra"},
		{"doctor", "extra"},
		{"debug", "extra"},
		{"sync-names", "extra"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			root, _ := buildRefinementTestRoot(t, srv.URL, "table")
			root.SetArgs(args)
			if err := root.Execute(); err == nil {
				t.Fatalf("%v unexpectedly succeeded with extra argument", args)
			}
			if requests.Load() != 0 {
				t.Fatalf("%v sent %d server requests", args, requests.Load())
			}
		})
	}
}

func TestProviderPrioritiesRejectsTrailingCharacters(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	root, _ := buildRefinementTestRoot(t, srv.URL, "table")
	t.Cleanup(func() {
		_ = providerPrioritiesCmd.Flags().Set("set", "")
		if f := providerPrioritiesCmd.Flags().Lookup("set"); f != nil {
			f.Changed = false
		}
	})
	root.SetArgs([]string{"provider", "priorities", "--set", "my-id:10abc"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid priority") {
		t.Fatalf("expected invalid priority error, got %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid priority sent %d requests", requests.Load())
	}
}

func TestVirtualModelCreateUpdateEscapeAndCompletions(t *testing.T) {
	var lastMethod, lastPath string
	var lastBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastMethod = r.Method
		lastPath = r.URL.EscapedPath()
		bodyBytes, _ := io.ReadAll(r.Body)
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &lastBody)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/admin/virtualmodels/") {
			_, _ = w.Write([]byte(`{"virtual_model_id":"vm/a","name":"Old","description":"Old desc","lb_strategy":"round-robin","api_shape":"openai","enabled":true,"upstreams":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"virtual_model_id":"vm-one"}]}`))
	}))
	defer srv.Close()

	// Create must mark --upstream required at Cobra level.
	if f := vmCreateCmd.Flags().Lookup("upstream"); f == nil || len(f.Annotations[cobra.BashCompOneRequiredFlag]) == 0 {
		t.Fatal("virtualmodel create --upstream should be marked required")
	}

	// Update must clear --description "" when explicitly passed and URL-escape ID.
	root, _ := buildRefinementTestRoot(t, srv.URL, "json")
	t.Cleanup(func() {
		for _, name := range []string{"name", "description", "strategy", "api-shape"} {
			_ = vmUpdateCmd.Flags().Set(name, "")
			if f := vmUpdateCmd.Flags().Lookup(name); f != nil {
				f.Changed = false
			}
		}
	})
	root.SetArgs([]string{"virtualmodel", "update", "vm/a", "--description", ""})
	if err := root.Execute(); err != nil {
		t.Fatalf("virtualmodel update: %v", err)
	}
	if lastMethod != http.MethodPut || lastPath != "/api/admin/virtualmodels/vm%2Fa" {
		t.Fatalf("unexpected request: %s %s", lastMethod, lastPath)
	}
	if desc, ok := lastBody["description"].(string); !ok || desc != "" {
		t.Fatalf("expected cleared description \"\", got %#v", lastBody["description"])
	}

	// Rename must have ValidArgsFunction and URL-escape old ID.
	if vmRenameCmd.ValidArgsFunction == nil {
		t.Fatal("virtualmodel rename missing ValidArgsFunction")
	}
	root, _ = buildRefinementTestRoot(t, srv.URL, "json")
	root.SetArgs([]string{"virtualmodel", "rename", "vm/a", "vm-b"})
	if err := root.Execute(); err != nil {
		t.Fatalf("virtualmodel rename: %v", err)
	}
	if lastMethod != http.MethodPost || lastPath != "/api/admin/virtualmodels/vm%2Fa/rename" {
		t.Fatalf("unexpected rename path: %s %s", lastMethod, lastPath)
	}
}

func TestLogsHonorsGlobalOutputJSONFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/logs/stream" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [2026-11-15T10:00:00Z] | backend | INFO | started\n\n"))
	}))
	defer srv.Close()

	root, out := buildRefinementTestRoot(t, srv.URL, "json")
	root.SetArgs([]string{"logs"})
	if err := root.Execute(); err != nil {
		t.Fatalf("logs -o json: %v", err)
	}
	if strings.Contains(out.String(), "Connected to log stream") {
		t.Fatalf("json mode should suppress banner, got %q", out.String())
	}
	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &entry); err != nil {
		t.Fatalf("expected valid NDJSON log entry, got %q: %v", out.String(), err)
	}
	if entry.Message != "started" || entry.Source != "backend" {
		t.Fatalf("unexpected log entry: %+v", entry)
	}
}

func TestDoctorAndDebugUseCanonicalLoginAndReadOnlyDB(t *testing.T) {
	// Doctor recommends omnillm provider login when zero providers exist.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/admin/status":
			_, _ = w.Write([]byte(`{"status":"healthy","uptime":"1s","modelCount":0}`))
		case "/api/admin/providers":
			_, _ = w.Write([]byte(`[]`))
		case "/api/admin/virtualmodels":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api/admin/auth-status":
			_, _ = w.Write([]byte(`{"status":"idle"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("OMNILLM_SERVER", srv.URL)
	t.Setenv("OMNILLM_API_KEY", "test-key")

	// First verify debug without a config dir or DB does not create database.sqlite.
	var dbgOut bytes.Buffer
	dbgCmd := &cobra.Command{Use: "debug"}
	dbgCmd.SetOut(&dbgOut)
	dbgCmd.SetErr(&dbgOut)
	if err := DebugCmd.RunE(dbgCmd, nil); err != nil {
		t.Fatalf("run debug without DB: %v", err)
	}
	dbPath := filepath.Join(tmpHome, ".config", "omnillm", "database.sqlite")
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("debug created database file unexpectedly at %s", dbPath)
	}
	if !strings.Contains(dbgOut.String(), "Database: NOT FOUND") {
		t.Fatalf("expected Database: NOT FOUND in debug output:\n%s", dbgOut.String())
	}
	if strings.Contains(dbgOut.String(), "omnillm auth") {
		t.Fatalf("debug should not suggest deprecated omnillm auth:\n%s", dbgOut.String())
	}

	// Prepare initialized local config so doctor reaches the missing-provider hint.
	configDir := filepath.Join(tmpHome, ".config", "omnillm")
	if err := database.InitializeDatabase(configDir); err != nil {
		t.Fatalf("init db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "api-key"), []byte("test-key"), 0o600); err != nil {
		database.StopAsyncWorkers()
		_ = database.GetDatabase().Close()
		t.Fatalf("write api-key: %v", err)
	}

	var docOut bytes.Buffer
	docCmd := &cobra.Command{Use: "doctor"}
	docCmd.SetOut(&docOut)
	docCmd.SetErr(&docOut)
	if err := runDoctor(docCmd, nil); err != nil {
		database.StopAsyncWorkers()
		_ = database.GetDatabase().Close()
		t.Fatalf("runDoctor: %v", err)
	}
	if !strings.Contains(docOut.String(), "omnillm provider login") || strings.Contains(docOut.String(), "omnillm auth") {
		database.StopAsyncWorkers()
		_ = database.GetDatabase().Close()
		t.Fatalf("doctor hint should reference omnillm provider login:\n%s", docOut.String())
	}

	// Debug lists tokens across non-copilot providers when DB exists.
	if err := database.NewProviderInstanceStore().Save(&database.ProviderInstanceRecord{
		InstanceID: "google-prod-1",
		ProviderID: "google",
		Name:       "Google Prod",
		Activated:  true,
	}); err != nil {
		database.StopAsyncWorkers()
		_ = database.GetDatabase().Close()
		t.Fatalf("save provider: %v", err)
	}
	if err := database.NewTokenStore().Save("google-prod-1", map[string]string{"access_token": "secret"}); err != nil {
		database.StopAsyncWorkers()
		_ = database.GetDatabase().Close()
		t.Fatalf("save token: %v", err)
	}
	// Close seeded DB workers before DebugCmd opens the DB cleanly.
	database.StopAsyncWorkers()
	_ = database.GetDatabase().Close()

	dbgOut.Reset()
	if err := DebugCmd.RunE(dbgCmd, nil); err != nil {
		t.Fatalf("run debug with DB: %v", err)
	}
	if !strings.Contains(dbgOut.String(), "google-prod-1") {
		t.Fatalf("expected non-copilot token instance google-prod-1 in debug output:\n%s", dbgOut.String())
	}
}

func TestCompletionConfigAndModelParameterHygiene(t *testing.T) {
	if CompletionCmd.Use != "completion <bash|zsh|fish|powershell>" {
		t.Fatalf("CompletionCmd.Use = %q, want required angle brackets", CompletionCmd.Use)
	}

	root, out := buildRefinementTestRoot(t, "", "table")
	root.SetArgs([]string{"completion", "bash"})
	if err := root.Execute(); err != nil {
		t.Fatalf("completion bash: %v", err)
	}
	if !strings.Contains(out.String(), "__omnillm") {
		t.Fatalf("completion did not write script to cmd.OutOrStdout(), got %d bytes", out.Len())
	}

	// provider login flag descriptions and completions
	if f := providerLoginCmd.Flags().Lookup("region"); f == nil || strings.Contains(f.Usage, "azure-openai") {
		t.Fatalf("provider login --region should not mention azure-openai: %v", f)
	}
	if f := providerLoginCmd.Flags().Lookup("endpoint"); f == nil || !strings.Contains(f.Usage, "azure-openai") {
		t.Fatalf("provider login --endpoint should mention azure-openai: %v", f)
	}
	if providerLoginCmd.Long == "" || providerLoginCmd.Example == "" {
		t.Fatal("provider login should document supported provider types and examples")
	}

	// provider model subcommands have ValidArgsFunction
	for _, sub := range []*cobra.Command{
		modelListCmd, modelRefreshCmd, modelToggleCmd,
		modelEnableCmd, modelDisableCmd, modelVersionGetCmd, modelVersionSetCmd,
	} {
		if sub.ValidArgsFunction == nil {
			t.Errorf("provider model %s missing ValidArgsFunction", sub.Name())
		}
	}

	// provider show accepts 0 or 1 arg (interactive TTY selection vs explicit reference)
	if err := providerShowCmd.Args(providerShowCmd, nil); err != nil {
		t.Errorf("provider show should allow 0 args for TTY selection: %v", err)
	}

	// config commands support ValidArgs, URL escaping, and cmd.InOrStdin()
	for _, sub := range []*cobra.Command{configGetCmd, configSetCmd, configImportCmd, configBackupCmd} {
		if len(sub.ValidArgs) == 0 {
			t.Errorf("config %s missing ValidArgs", sub.Name())
		}
	}
	var gotConfigPath, gotContent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotConfigPath = r.URL.EscapedPath()
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(body, &payload)
		gotContent = payload["content"]
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	root, _ = buildRefinementTestRoot(t, srv.URL, "table")
	t.Cleanup(func() {
		_ = configSetCmd.Flags().Set("stdin", "false")
		if f := configSetCmd.Flags().Lookup("stdin"); f != nil {
			f.Changed = false
		}
	})
	root.SetIn(strings.NewReader(`{"ok":true}`))
	root.SetArgs([]string{"config", "set", "bad name", "--stdin"})
	if err := root.Execute(); err != nil {
		t.Fatalf("config set --stdin: %v", err)
	}
	if gotConfigPath != "/api/admin/config/bad%20name" || gotContent != `{"ok":true}` {
		t.Fatalf("config set path=%q content=%q", gotConfigPath, gotContent)
	}
}
