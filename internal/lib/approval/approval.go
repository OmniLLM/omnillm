// Package approval provides manual approval workflow for requests
package approval

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

func AwaitApproval() error {
	log.Info().Msg("Manual approval required for this request")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Accept incoming request? (y/N): ")

	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))

	if response == "y" || response == "yes" {
		log.Info().Msg("Request approved by user")
		return nil
	}

	log.Warn().Msg("Request rejected by user")
	return fmt.Errorf("request rejected by user")
}
