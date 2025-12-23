package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	searchPage    int
	searchPerPage int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for content",
	Long:  `Search the server for content matching the given query.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")

		result, err := apiClient.Search(query, searchPage, searchPerPage)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		switch getOutputFormat() {
		case "json":
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		case "plain":
			for _, r := range result.Results {
				fmt.Printf("%s\n%s\n\n", r.Title, r.URL)
			}
		default:
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "TITLE\tURL\tSCORE\n")
			for _, r := range result.Results {
				title := r.Title
				if len(title) > 50 {
					title = title[:47] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\t%.2f\n", title, r.URL, r.Score)
			}
			w.Flush()
			fmt.Printf("\nTotal: %d results\n", result.TotalCount)
		}
		return nil
	},
}

func init() {
	searchCmd.Flags().IntVarP(&searchPage, "page", "p", 1, "page number")
	searchCmd.Flags().IntVarP(&searchPerPage, "limit", "l", 10, "results per page")
}
