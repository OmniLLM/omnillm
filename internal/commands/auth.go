package commands

import "github.com/spf13/cobra"

var AuthCmd = &cobra.Command{
	Use:   "auth [type]",
	Short: "Authenticate and add a provider through the OmniLLM backend",
	Long:  "Authenticate and add a provider through the OmniLLM backend.\n\nIf no provider type is supplied, an interactive prompt lets you choose one.\nSupported types match \"omnillm provider add\": " + supportedAuthProviderTypesSummary + ".",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerType, err := resolveAuthProviderType(args)
		if err != nil {
			return err
		}
		if err := promptForProviderAuth(cmd, providerType); err != nil {
			return err
		}
		return authAndCreateProvider(cmd, providerType)
	},
}

func resolveAuthProviderType(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	return selectProviderTypeInteractive()
}

func init() {
	addProviderAuthFlags(AuthCmd)
}
