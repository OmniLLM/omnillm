package routes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"omnillm/internal/lib/ratelimit"
)

// Log subscriber for SSE streaming
type logSubscriber struct {
	ch   chan string
	done chan struct{}
}

var (
	logSubscribersMu sync.RWMutex
	logSubscribers   = make(map[*logSubscriber]struct{})
	currentLogLevel  atomic.Int32 // stores zerolog.Level (int32)
	serverStartTime  = time.Now()
	adminStatus      = newAdminStatus()

	// Active OAuth device code flow state
	activeAuthFlowMu sync.RWMutex
	activeAuthFlow   *authFlowState
)

type adminStatusState struct {
	mu             sync.RWMutex
	rateLimiter    *ratelimit.RateLimiter
	manualApproval bool
}

func newAdminStatus() *adminStatusState {
	return &adminStatusState{
		rateLimiter: ratelimit.NewRateLimiter(0, false),
	}
}

// ConfigureAdminStatus updates the global adminStatus based on supplied options
func ConfigureAdminStatus(options ChatCompletionOptions) {
	adminStatus.mu.Lock()
	defer adminStatus.mu.Unlock()

	if options.RateLimiter != nil {
		adminStatus.rateLimiter = options.RateLimiter
	} else {
		adminStatus.rateLimiter = ratelimit.NewRateLimiter(0, false)
	}
	adminStatus.manualApproval = options.ManualApproval
}

func getAdminStatusSnapshot() (bool, *ratelimit.RateLimiter) {
	adminStatus.mu.RLock()
	defer adminStatus.mu.RUnlock()
	return adminStatus.manualApproval, adminStatus.rateLimiter
}

type authFlowState struct {
	ProviderID     string             `json:"providerId"`
	Status         string             `json:"status"` // pending, awaiting_user, complete, error
	InstructionURL string             `json:"instructionURL,omitempty"`
	UserCode       string             `json:"userCode,omitempty"`
	Error          string             `json:"error,omitempty"`
	deviceCode     string             // internal, not exposed
	codeVerifier   string             // internal PKCE verifier for Alibaba OAuth
	cancelFn       context.CancelFunc // cancels the background polling goroutine
}

// BroadcastLog sends a log message to all SSE subscribers
func BroadcastLog(level, message string) {
	timestamp := time.Now().Format(time.RFC3339)
	BroadcastLogLine(fmt.Sprintf("[%s] | backend | %s | %s", timestamp, strings.ToUpper(level), message))
}

// BroadcastLogLine sends a preformatted log line to all SSE subscribers.
func BroadcastLogLine(line string) {
	logSubscribersMu.RLock()
	defer logSubscribersMu.RUnlock()

	data := formatSSEData(line)
	for sub := range logSubscribers {
		select {
		case sub.ch <- data:
		default:
			// subscriber too slow, skip
		}
	}
}

func formatSSEData(message string) string {
	var builder strings.Builder
	for _, line := range strings.Split(strings.TrimRight(message, "\n"), "\n") {
		builder.WriteString("data: ")
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	return builder.String()
}
