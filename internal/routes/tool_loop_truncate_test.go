package routes

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// truncateToolLoopValue previously sliced at a fixed 400-byte offset, severing
// any multi-byte character straddling it and emitting a log field that was not
// valid UTF-8. Tool results routinely carry CJK, emoji, and accented text.
func TestTruncateToolLoopValue_MultiByteBoundaryIsValidUTF8(t *testing.T) {
	cases := map[string]string{
		"3-byte CJK":    strings.Repeat("世", 300),
		"4-byte emoji":  strings.Repeat("🚀", 200),
		"2-byte accent": strings.Repeat("é", 400),
		"mixed":         strings.Repeat("a世🚀é", 100),
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			got := truncateToolLoopValue(input)

			if !utf8.ValidString(got) {
				t.Fatalf("truncated preview is not valid UTF-8: % x", got[len(got)-8:])
			}
			body := strings.TrimSuffix(got, "...(truncated)")
			if len(body) > toolLoopLogValueLimit {
				t.Fatalf("preview body is %d bytes, exceeds limit %d", len(body), toolLoopLogValueLimit)
			}
			if !strings.HasSuffix(got, "...(truncated)") {
				t.Fatal("oversized value is missing the truncation marker")
			}
		})
	}
}

func TestTruncateToolLoopValue_ASCIITruncatesAtLimit(t *testing.T) {
	input := strings.Repeat("a", toolLoopLogValueLimit+50)

	got := truncateToolLoopValue(input)

	want := strings.Repeat("a", toolLoopLogValueLimit) + "...(truncated)"
	if got != want {
		t.Fatalf("ASCII truncation produced %d bytes, want %d + marker", len(got), toolLoopLogValueLimit)
	}
}

func TestTruncateToolLoopValue_WithinLimitUnchanged(t *testing.T) {
	for name, input := range map[string]string{
		"ascii":     strings.Repeat("a", toolLoopLogValueLimit),
		"multibyte": strings.Repeat("世", 50),
		"short":     "hello",
	} {
		t.Run(name, func(t *testing.T) {
			got := truncateToolLoopValue(input)
			if got != input {
				t.Fatalf("value within limit was modified:\n got: %q\nwant: %q", got, input)
			}
			if strings.Contains(got, "...(truncated)") {
				t.Fatal("value within limit should carry no truncation marker")
			}
		})
	}
}

func TestTruncateToolLoopValue_EmptyAndWhitespace(t *testing.T) {
	for _, input := range []string{"", "   ", "\n\t "} {
		if got := truncateToolLoopValue(input); got != "" {
			t.Fatalf("truncateToolLoopValue(%q) = %q, want empty", input, got)
		}
	}
}

// Sweep lengths around the boundary so no rune alignment slips through.
func TestTruncateToolLoopValue_BoundarySweep(t *testing.T) {
	for n := toolLoopLogValueLimit - 10; n <= toolLoopLogValueLimit+10; n++ {
		input := strings.Repeat("世", n)
		got := truncateToolLoopValue(input)
		if !utf8.ValidString(got) {
			t.Fatalf("invalid UTF-8 for input of %d runes", n)
		}
	}
}
