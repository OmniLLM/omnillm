package openaicompat

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

// The aliasing regression.
//
// cappedBody previously used append(b[:limit], marker...), which reuses b's
// backing array when cap(b) > limit. Callers log the body and then transmit
// that same buffer, so truncating for diagnostics corrupted the bytes actually
// sent upstream. The corruption was invisible in the log, which rendered the
// intended truncated text either way.
func TestCappedBody_DoesNotMutateCaller(t *testing.T) {
	// Spare capacity is what arms the bug; json.Marshal output has it.
	payload := make([]byte, traceBodyLimit+64, traceBodyLimit+4096)
	for i := range payload {
		payload[i] = 'A'
	}
	original := append([]byte(nil), payload...)

	_ = cappedBody(payload)

	if !bytes.Equal(payload, original) {
		t.Fatalf("cappedBody mutated the caller's buffer\n got: %q\nwant: %q",
			payload[traceBodyLimit-4:traceBodyLimit+20],
			original[traceBodyLimit-4:traceBodyLimit+20])
	}
}

// The transmitted payload must be byte-identical to what was marshalled.
func TestCappedBody_TransmittedPayloadUnchanged(t *testing.T) {
	payload := []byte(strings.Repeat("x", traceBodyLimit*2))
	original := append([]byte(nil), payload...)

	logged := cappedBody(payload)

	if bytes.Contains(payload, []byte(truncationMarker)) {
		t.Fatal("truncation marker leaked into the outbound payload")
	}
	if !bytes.Equal(payload, original) {
		t.Fatal("outbound payload differs from the marshalled bytes")
	}
	if !bytes.Contains(logged, []byte(truncationMarker)) {
		t.Fatal("logged value is missing the truncation marker")
	}
}

func TestCappedBody_RepeatedTruncationIsStable(t *testing.T) {
	payload := []byte(strings.Repeat("y", traceBodyLimit*3))
	original := append([]byte(nil), payload...)

	first := append([]byte(nil), cappedBody(payload)...)
	second := cappedBody(payload)

	if !bytes.Equal(first, second) {
		t.Fatalf("repeated truncation diverged:\nfirst:  %q\nsecond: %q", first, second)
	}
	if !bytes.Equal(payload, original) {
		t.Fatal("repeated truncation mutated the source buffer")
	}
}

func TestCappedBody_WithinLimitUnchanged(t *testing.T) {
	payload := []byte(strings.Repeat("z", traceBodyLimit))

	got := cappedBody(payload)

	if !bytes.Equal(got, payload) {
		t.Fatal("payload at the limit should be returned unchanged")
	}
	if bytes.Contains(got, []byte(truncationMarker)) {
		t.Fatal("payload at the limit should carry no truncation marker")
	}
}

// The encoding regression: a multi-byte rune straddling the limit must not be
// severed.
func TestCappedBody_MultiByteBoundaryIsValidUTF8(t *testing.T) {
	// 3-byte runes do not divide evenly into 1024, guaranteeing a straddle.
	payload := []byte(strings.Repeat("世", 600))

	got := cappedBody(payload)

	body := bytes.TrimSuffix(got, []byte(truncationMarker))
	if !utf8.Valid(body) {
		t.Fatalf("truncated body is not valid UTF-8: % x", body[len(body)-8:])
	}
	if len(body) > traceBodyLimit {
		t.Fatalf("truncated body is %d bytes, exceeds limit %d", len(body), traceBodyLimit)
	}
}

// Sweep every boundary offset so no rune alignment slips through.
func TestUTF8SafeCut_AllOffsetsValidAndBounded(t *testing.T) {
	inputs := map[string]string{
		"3-byte CJK":    strings.Repeat("世", 400),
		"4-byte emoji":  strings.Repeat("🚀", 300),
		"2-byte accent": strings.Repeat("é", 500),
		"mixed":         strings.Repeat("a世🚀é", 150),
	}

	for name, s := range inputs {
		t.Run(name, func(t *testing.T) {
			b := []byte(s)
			for limit := 0; limit <= len(b); limit++ {
				cut := utf8SafeCut(b, limit)
				if cut > limit {
					t.Fatalf("cut %d exceeds limit %d", cut, limit)
				}
				if !utf8.Valid(b[:cut]) {
					t.Fatalf("cut at %d (limit %d) produced invalid UTF-8", cut, limit)
				}
			}
		})
	}
}

func TestCappedBody_ASCIITruncatesAtLimit(t *testing.T) {
	payload := []byte(strings.Repeat("q", traceBodyLimit+100))

	got := cappedBody(payload)

	want := strings.Repeat("q", traceBodyLimit) + truncationMarker
	if string(got) != want {
		t.Fatalf("ASCII truncation = %d bytes, want exactly %d + marker", len(got), traceBodyLimit)
	}
}
