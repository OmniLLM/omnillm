package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDeviceTokenPollingStopsOnCancellation(t *testing.T) {
	oldClient := httpClient
	started := make(chan struct{}, 1)
	httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		started <- struct{}{}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"error":"authorization_pending"}`)), Header: make(http.Header), Request: request}, nil
	})}
	t.Cleanup(func() { httpClient = oldClient })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := PollAccessTokenContext(ctx, &DeviceCodeResponse{DeviceCode: "code", ExpiresIn: 60, Interval: 20})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("poll did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("poll error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("poll did not stop after cancellation")
	}
}
