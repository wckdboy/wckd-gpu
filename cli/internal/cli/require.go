package cli

import (
	"github.com/spf13/cobra"
	"github.com/wckdboy/wckd-gpu/cli/internal/offer"
)

func requireOverride(cmd *cobra.Command, raw string) (*[]string, error) {
	if cmd == nil || !cmd.Flags().Changed("require-regions") {
		return nil, nil
	}
	parts := offer.SplitList(raw)
	if _, err := offer.ParseRequire(parts); err != nil {
		return nil, err
	}
	return &parts, nil
}
