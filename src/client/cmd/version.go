package cmd

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/apimgr/search/src/client/api"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("%s v%s (%s) built %s\n", getBinaryName(), Version, CommitID, BuildDate)

		serverAddr := viper.GetString("server.address")
		if server != "" {
			serverAddr = server
		}

		if serverAddr != "" {
			fmt.Printf("\nServer: %s\n", serverAddr)

			client := api.NewClient(serverAddr, "", 5)
			client.HTTPClient = &http.Client{Timeout: 5 * time.Second}
			if serverVer, err := client.GetVersion(); err == nil {
				compat := "compatible"
				if serverVer != Version {
					compat = "different"
				}
				fmt.Printf("Server Version: v%s (%s)\n", serverVer, compat)
			}
		}

		fmt.Printf("\nBuild Info:\n")
		fmt.Printf("  Go: %s\n", runtime.Version())
		fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("  Commit: %s\n", CommitID)
		fmt.Printf("  Date: %s\n", BuildDate)

		return nil
	},
}
