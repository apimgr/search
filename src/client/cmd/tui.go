package cmd

import (
	"github.com/spf13/cobra"

	"github.com/apimgr/search/src/client/tui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive TUI mode",
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run(apiClient)
	},
}
