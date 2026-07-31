package openai

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// CallbackResult carries the outcome of the browser redirect.
type CallbackResult struct {
	Code  string
	State string
	Err   error
}

// callbackMu ensures only one listener binds port 1455 at a time.
var callbackMu sync.Mutex

// ListenForCallback starts a short-lived HTTP server on the whitelisted
// callback port and blocks until the browser redirect arrives, the context is
// cancelled, or the timeout elapses.
//
// The redirect URI is fixed by the OAuth client registration, so this cannot
// be folded into the main OmniLLM server port the way the antigravity provider
// does it.
func ListenForCallback(ctx context.Context, expectedState string, timeout time.Duration) (*CallbackResult, error) {
	callbackMu.Lock()
	defer callbackMu.Unlock()

	addr := fmt.Sprintf("127.0.0.1:%d", CallbackPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("openai: cannot bind %s for the OAuth callback "+
			"(the sign-in redirect requires this exact port — free it and retry): %w", addr, err)
	}

	resultCh := make(chan *CallbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		code, state, errParam := q.Get("code"), q.Get("state"), q.Get("error")

		var res *CallbackResult
		switch {
		case errParam != "":
			res = &CallbackResult{Err: fmt.Errorf("openai: sign-in denied: %s", errParam)}
		case code == "" || state == "":
			res = &CallbackResult{Err: fmt.Errorf("openai: callback missing code or state")}
		case state != expectedState:
			// Mismatched state means the response does not belong to the flow
			// we started; reject rather than exchanging the code.
			res = &CallbackResult{Err: fmt.Errorf("openai: state mismatch — possible CSRF, please retry")}
		default:
			res = &CallbackResult{Code: code, State: state}
		}

		renderCallbackPage(w, res.Err)

		select {
		case resultCh <- res:
		default:
		}
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Debug().Err(serveErr).Msg("OpenAI: OAuth callback server stopped")
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case res := <-resultCh:
		if res.Err != nil {
			return nil, res.Err
		}
		return res, nil
	case <-timer.C:
		return nil, fmt.Errorf("openai: timed out after %s waiting for the browser sign-in", timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// renderCallbackPage writes the small self-closing page the user lands on.
func renderCallbackPage(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>OpenAI — Sign-in failed</title></head><body>
<p style="color:red">Sign-in failed: %s</p>
<p><button onclick="window.close()">Close</button></p>
</body></html>`, err.Error())
		return
	}
	fmt.Fprint(w, `<!DOCTYPE html><html><head><title>OpenAI — Signed in</title></head><body>
<p>Signed in successfully. You can close this window.</p>
<script>setTimeout(function(){ window.close(); }, 1000);</script>
</body></html>`)
}
