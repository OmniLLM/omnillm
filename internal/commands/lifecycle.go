package commands

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"omnillm/internal/lifecycle"
)

const gracefulStopTimeout = 20 * time.Second

var StopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running LLM proxy server",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := lifecycle.DefaultPath()
		if err != nil {
			return err
		}
		if err := lifecycle.Stop(path, gracefulStopTimeout); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "OmniLLM server stopped.")
		return nil
	},
}

var RestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the LLM proxy server",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := lifecycle.DefaultPath()
		if err != nil {
			return err
		}
		if err := lifecycle.Stop(path, gracefulStopTimeout); err != nil && !errors.Is(err, lifecycle.ErrNotRunning) {
			return fmt.Errorf("stop existing server: %w", err)
		}
		return StartCmd.RunE(cmd, args)
	},
}

func init() {
	addStartFlags(RestartCmd)
}
