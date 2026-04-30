package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestTableRender(t *testing.T) {
	table := NewTable("NAME", "VALUE")
	table.AddRow("alpha", "1")
	table.AddRow("beta", "22")

	var out bytes.Buffer
	if err := table.Render(&out); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	rendered := out.String()
	for _, want := range []string{"NAME", "VALUE", "alpha", "beta", "22"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered table missing %q:\n%s", want, rendered)
		}
	}
}

func TestPrintEmpty(t *testing.T) {
	var out bytes.Buffer
	if err := PrintEmpty(&out, "chat sessions"); err != nil {
		t.Fatalf("PrintEmpty returned error: %v", err)
	}
	if got := out.String(); got != "No chat sessions.\n" {
		t.Fatalf("PrintEmpty output = %q, want %q", got, "No chat sessions.\n")
	}
}

func TestPrintSection(t *testing.T) {
	var out bytes.Buffer
	if err := PrintSection(&out, "Status"); err != nil {
		t.Fatalf("PrintSection returned error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Status\n") || !strings.Contains(got, "──────") {
		t.Fatalf("unexpected section output:\n%s", got)
	}
}

func TestConfirmReadsFromCommandStreams(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("y\n"))
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if !Confirm(cmd, "Delete provider?") {
		t.Fatal("Confirm returned false, want true")
	}
	if got := stderr.String(); got != "Delete provider? [y/N] " {
		t.Fatalf("stderr prompt = %q, want %q", got, "Delete provider? [y/N] ")
	}
}
