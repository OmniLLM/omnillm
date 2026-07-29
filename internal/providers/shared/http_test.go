package shared

import (
	"testing"
	"time"
)

func TestTimeoutFromEnv(t *testing.T) {
	const key = "OMNILLM_TEST_TIMEOUT"
	def := 120 * time.Second

	tests := []struct {
		name string
		set  bool
		val  string
		want time.Duration
	}{
		{name: "unset falls back to default", set: false, want: def},
		{name: "empty falls back to default", set: true, val: "", want: def},
		{name: "whitespace falls back to default", set: true, val: "   ", want: def},
		{name: "go duration seconds", set: true, val: "240s", want: 240 * time.Second},
		{name: "go duration minutes", set: true, val: "5m", want: 5 * time.Minute},
		{name: "bare seconds", set: true, val: "300", want: 300 * time.Second},
		{name: "bare seconds with padding", set: true, val: " 90 ", want: 90 * time.Second},
		{name: "garbage falls back to default", set: true, val: "soon", want: def},
		{name: "zero falls back to default", set: true, val: "0", want: def},
		{name: "negative falls back to default", set: true, val: "-30s", want: def},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(key, tt.val)
			}
			if got := TimeoutFromEnv(key, def); got != tt.want {
				t.Fatalf("TimeoutFromEnv(%q) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

// The /responses budget must exceed the chat budget: reasoning models can
// think for minutes before emitting response headers, and sharing the
// chat-sized cap is what made those requests structurally unsatisfiable.
func TestResponsesTimeoutExceedsRequestTimeout(t *testing.T) {
	if DefaultResponsesTimeout <= DefaultRequestTimeout {
		t.Fatalf("DefaultResponsesTimeout (%v) must exceed DefaultRequestTimeout (%v)",
			DefaultResponsesTimeout, DefaultRequestTimeout)
	}
}

func TestDefaultStreamClientHasNoTimeout(t *testing.T) {
	// SSE connections are long-lived; a client Timeout would sever them
	// mid-stream regardless of how healthy the upstream is.
	if got := DefaultStreamClient().Timeout; got != 0 {
		t.Fatalf("DefaultStreamClient().Timeout = %v, want 0", got)
	}
}

func TestDefaultHTTPTransportPoolSizing(t *testing.T) {
	tr := DefaultHTTPTransport()
	// Go's default MaxIdleConnsPerHost is 2, which starves us under
	// concurrent load — the whole reason this helper exists.
	if tr.MaxIdleConnsPerHost <= 2 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want > 2", tr.MaxIdleConnsPerHost)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 = false, want true")
	}
	if tr.Proxy == nil {
		t.Fatal("Proxy = nil, want ProxyFromEnvironment")
	}
}
