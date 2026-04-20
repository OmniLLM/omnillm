package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var ChatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Interactive chat mode",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Chat command not implemented yet")
	},
}
