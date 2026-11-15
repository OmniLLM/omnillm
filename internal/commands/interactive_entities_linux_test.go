//go:build linux

package commands

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

// TestInteractiveEntityChild runs the real command against a pseudo-terminal.
func TestInteractiveEntityChild(t *testing.T) {
	command := os.Getenv("OMNILLM_TEST_INTERACTIVE_COMMAND")
	if command == "" {
		return
	}
	root := &cobra.Command{Use: "omnillm", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("server", os.Getenv("OMNILLM_TEST_SERVER"), "")
	root.PersistentFlags().String("api-key", "", "")
	root.PersistentFlags().String("output", "table", "")
	root.AddCommand(cloneCommandTree(ProviderCmd), cloneCommandTree(VirtualModelCmd))
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetArgs(strings.Fields(command))
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestInteractiveEntityCommandsShowNamesAndUseIDs(t *testing.T) {
	for _, test := range []struct {
		name        string
		command     string
		search      string
		requestPath string
		wantLabels  []string
	}{
		{
			name: "provider show", command: "provider show", search: "SECONDALIAS",
			requestPath: "/api/admin/providers/provider-second/metadata",
			wantLabels:  []string{"Shared Provider", "firstalias", "secondalias", "provider-first", "provider-second"},
		},
		{
			name: "provider enable", command: "provider enable", search: "SECONDALIAS",
			requestPath: "/api/admin/providers/provider-second/activate",
			wantLabels:  []string{"Shared Provider", "firstalias", "secondalias", "provider-first", "provider-second"},
		},
		{
			name: "virtual model get", command: "virtualmodel get", search: "SECOND MODEL",
			requestPath: "/api/admin/virtualmodels/vm-second",
			wantLabels:  []string{"First Model", "Second Model", "vm-first", "vm-second"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan string, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.URL.Path
				switch r.URL.Path {
				case "/api/admin/providers":
					fmt.Fprint(w, `[{"id":"provider-first","name":"Shared Provider","alias":"firstalias","config":{"token":"SECRET"}},{"id":"provider-second","name":"Shared Provider","subtitle":"secondalias"}]`)
				case "/api/admin/virtualmodels":
					fmt.Fprint(w, `{"data":[{"virtual_model_id":"vm-first","name":"First Model"},{"virtual_model_id":"vm-second","name":"Second Model"}]}`)
				case "/api/admin/virtualmodels/vm-second":
					fmt.Fprint(w, `{"virtual_model_id":"vm-second","name":"Second Model"}`)
				case "/api/admin/providers/provider-second/activate":
					fmt.Fprint(w, `{"success":true}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			output, err := runInteractiveEntityChild(t, server.URL, test.command, test.search)
			if err != nil {
				t.Fatalf("interactive command: %v\noutput: %s", err, output)
			}
			for _, label := range test.wantLabels {
				if !strings.Contains(output, label) {
					t.Errorf("output missing %q: %s", label, output)
				}
			}
			if strings.Contains(output, "SECRET") {
				t.Errorf("output leaked unrelated configuration: %s", output)
			}
			found := false
			close(requests)
			for path := range requests {
				if path == test.requestPath {
					found = true
				}
			}
			if !found {
				t.Errorf("selected ID did not reach %s", test.requestPath)
			}
		})
	}
}

func TestInteractiveEntityCancellationDoesNotModifyProvider(t *testing.T) {
	requests := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.URL.Path
		fmt.Fprint(w, `[{"id":"first","name":"First"},{"id":"second","name":"Second"}]`)
	}))
	defer server.Close()
	output, err := runInteractiveEntityChild(t, server.URL, "provider enable", "\x03")
	if err == nil {
		t.Fatalf("cancelled command succeeded: %s", output)
	}
	close(requests)
	for request := range requests {
		if request != "GET /api/admin/providers" {
			t.Errorf("unexpected request after cancellation: %s", request)
		}
	}
}

func runInteractiveEntityChild(t *testing.T, server, command, input string) (string, error) {
	t.Helper()
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("pseudo-terminal unavailable: %v", err)
	}
	master := os.NewFile(uintptr(fd), "ptmx")
	defer master.Close()
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInteractiveEntityChild$")
	cmd.Env = append(os.Environ(), "OMNILLM_API_KEY=", "OMNILLM_TEST_INTERACTIVE_COMMAND="+command, "OMNILLM_TEST_SERVER="+server)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = slave.Close()

	ready := make(chan struct{}, 1)
	result := make(chan string, 1)
	go func() {
		var output strings.Builder
		buf := make([]byte, 4096)
		signaled := false
		for {
			n, readErr := master.Read(buf)
			output.Write(buf[:n])
			if !signaled && strings.Contains(output.String(), "Search:") {
				signaled = true
				ready <- struct{}{}
			}
			if readErr != nil {
				result <- output.String()
				return
			}
		}
	}()
	select {
	case <-ready:
		_, _ = io.WriteString(master, input+"\r")
	case <-ctx.Done():
	}
	err = cmd.Wait()
	return <-result, err
}
