package commands

import "github.com/spf13/cobra"

// CompletionCmd generates shell completion scripts.
var CompletionCmd = &cobra.Command{
	Use:   "completion <bash|zsh|fish|powershell>",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for omnillm.

To load completions in your current shell session:

  # Bash
  source <(omnillm completion bash)

  # Zsh
  source <(omnillm completion zsh)

  # Fish
  omnillm completion fish | source

  # PowerShell
  omnillm completion powershell | Out-String | Invoke-Expression
`,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		root := cmd.Root()
		out := cmd.OutOrStdout()
		switch args[0] {
		case "bash":
			return root.GenBashCompletion(out)
		case "zsh":
			return root.GenZshCompletion(out)
		case "fish":
			return root.GenFishCompletion(out, true)
		case "powershell":
			return root.GenPowerShellCompletionWithDesc(out)
		}
		return nil
	},
}
