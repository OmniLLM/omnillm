package routes

import (
	"errors"
	"strings"
	"testing"
)

func TestReadGatewayRequestBodyBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		size      int
		oversized bool
	}{
		{name: "below", size: maxGatewayRequestBodyBytes - 1},
		{name: "at", size: maxGatewayRequestBodyBytes},
		{name: "above", size: maxGatewayRequestBodyBytes + 1, oversized: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := readGatewayRequestBody(strings.NewReader(strings.Repeat("x", tc.size)))
			if tc.oversized {
				if !errors.Is(err, errRequestBodyTooLarge) {
					t.Fatalf("expected oversized error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected read error: %v", err)
			}
			if len(body) != tc.size {
				t.Fatalf("expected %d bytes, got %d", tc.size, len(body))
			}
		})
	}
}

func TestReadGatewayRequestBodyPropagatesReadError(t *testing.T) {
	expected := errors.New("read failed")
	_, err := readGatewayRequestBody(ioErrorReader{err: expected})
	if !errors.Is(err, expected) {
		t.Fatalf("expected read error, got %v", err)
	}
}

type ioErrorReader struct{ err error }

func (r ioErrorReader) Read([]byte) (int, error) { return 0, r.err }
