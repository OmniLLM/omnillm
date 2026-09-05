//go:build darwin

package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDarwinProcessExecutableIdentifiesCurrentProcess(t *testing.T) {
	got, err := processExecutable(os.Getpid())
	if err != nil {
		t.Fatalf("identify current process executable: %v", err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("get current executable: %v", err)
	}
	want, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatalf("resolve current executable: %v", err)
	}
	if got != want {
		t.Fatalf("process executable = %q, want %q", got, want)
	}
}

func TestParseDarwinExecutable(t *testing.T) {
	data := []byte("/tmp/omnillm\x00ignored")

	got, err := parseDarwinExecutable(data)
	if err != nil {
		t.Fatalf("parse executable: %v", err)
	}
	if got != "/tmp/omnillm" {
		t.Fatalf("executable = %q, want /tmp/omnillm", got)
	}
}

func TestParseDarwinExecutableRejectsEmptyData(t *testing.T) {
	tests := map[string][]byte{
		"empty":      nil,
		"nul prefix": {0},
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseDarwinExecutable(data); err == nil {
				t.Fatal("parse malformed process path succeeded")
			}
		})
	}
}
