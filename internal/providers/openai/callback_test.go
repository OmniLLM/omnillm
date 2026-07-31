package openai

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// callbackURL builds a request against the fixed local callback endpoint.
func callbackURL(query string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s?%s", CallbackPort, CallbackPath, query)
}

// waitForListener polls until the callback port accepts connections, so tests
// do not race the server goroutine.
func waitForListener(t *testing.T) {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", CallbackPort)
	for i := 0; i < 100; i++ {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("callback listener never became reachable")
}

func TestListenForCallbackCapturesCode(t *testing.T) {
	type outcome struct {
		res *CallbackResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := ListenForCallback(context.Background(), "state-ok", 10*time.Second)
		done <- outcome{res, err}
	}()

	waitForListener(t)

	resp, err := http.Get(callbackURL("code=the-code&state=state-ok"))
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Signed in successfully") {
		t.Errorf("expected a success page, got %q", string(body))
	}

	out := <-done
	if out.err != nil {
		t.Fatalf("ListenForCallback: %v", out.err)
	}
	if out.res.Code != "the-code" {
		t.Errorf("Code = %q, want the-code", out.res.Code)
	}
}

func TestListenForCallbackRejectsStateMismatch(t *testing.T) {
	// A mismatched state means the redirect belongs to a different flow;
	// exchanging that code would be a CSRF hole.
	done := make(chan error, 1)
	go func() {
		_, err := ListenForCallback(context.Background(), "expected-state", 10*time.Second)
		done <- err
	}()

	waitForListener(t)

	resp, err := http.Get(callbackURL("code=abc&state=attacker-state"))
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	resp.Body.Close()

	err = <-done
	if err == nil {
		t.Fatal("expected an error on state mismatch")
	}
	if !strings.Contains(err.Error(), "state mismatch") {
		t.Errorf("error = %v, want a state-mismatch error", err)
	}
}

func TestListenForCallbackSurfacesProviderError(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		_, err := ListenForCallback(context.Background(), "s", 10*time.Second)
		done <- err
	}()

	waitForListener(t)

	resp, err := http.Get(callbackURL("error=access_denied"))
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	resp.Body.Close()

	err = <-done
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Errorf("error = %v, want it to surface access_denied", err)
	}
}

func TestListenForCallbackTimesOut(t *testing.T) {
	start := time.Now()
	_, err := ListenForCallback(context.Background(), "s", 150*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want a timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("timeout took %s; the deadline was not honoured", elapsed)
	}
}

func TestListenForCallbackHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := ListenForCallback(ctx, "s", time.Minute)
		done <- err
	}()

	waitForListener(t)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error after cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenForCallback ignored context cancellation")
	}
}

func TestListenForCallbackReleasesPortBetweenRuns(t *testing.T) {
	// The port must be freed on every exit path, otherwise a timed-out attempt
	// would permanently block sign-in until restart.
	for i := 0; i < 2; i++ {
		if _, err := ListenForCallback(context.Background(), "s", 100*time.Millisecond); err == nil {
			t.Fatal("expected a timeout")
		} else if strings.Contains(err.Error(), "cannot bind") {
			t.Fatalf("attempt %d could not bind: the previous run leaked the port", i+1)
		}
	}
}
