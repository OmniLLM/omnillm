package openaicompat

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"omnillm/internal/cif"
)

type failingStreamBody struct {
	err error
}

func (b failingStreamBody) Read([]byte) (int, error) { return 0, b.err }
func (failingStreamBody) Close() error               { return nil }

func TestSSEScannerErrorsIncludeContextFields(t *testing.T) {
	parsers := map[string]func(context.Context, io.ReadCloser, chan cif.CIFStreamEvent){
		"chat completions": ParseSSE,
		"responses":        ParseResponsesSSE,
	}

	for name, parse := range parsers {
		t.Run(name, func(t *testing.T) {
			var output strings.Builder
			logger := zerolog.New(&output).With().Str("request_id", "request-123").Logger()
			ctx := logger.WithContext(context.Background())
			events := make(chan cif.CIFStreamEvent, 1)

			parse(ctx, failingStreamBody{err: errors.New("upstream read failed")}, events)

			if !strings.Contains(output.String(), `"request_id":"request-123"`) {
				t.Fatalf("scanner error log missing request_id: %s", output.String())
			}
		})
	}
}
