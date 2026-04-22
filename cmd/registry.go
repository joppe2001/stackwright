package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Registry utilities (list, search, share)",
}

var registryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all entries in the merged registry (bundled + synced + local)",
	RunE: func(_ *cobra.Command, _ []string) error {
		r := registry.Load(registry.LoadOptions{Offline: FlagOffline})
		fmt.Fprintf(os.Stdout, "source: %s  entries: %d  local: %d",
			r.Source, len(r.Bundle.Entries), r.LocalEntries)
		if r.Source == registry.SourceCache {
			fmt.Fprintf(os.Stdout, "  cache-age: %s", r.CacheAge.Round(1e9))
		}
		if r.NetworkError != nil {
			fmt.Fprintf(os.Stdout, "\n(network error: %v)", r.NetworkError)
		}
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout)

		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SLUG\tCATEGORY\tNAME\tCLI\tAUTH")
		for _, e := range r.Bundle.Entries {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				e.Slug, e.Category, e.Name, cliSummary(e), authSummary(e))
		}
		return tw.Flush()
	},
}

var registrySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Fuzzy-search the registry and print ranked matches",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		r := registry.Load(registry.LoadOptions{Offline: FlagOffline})
		hits := r.Bundle.Search(args[0], "")
		if len(hits) == 0 {
			fmt.Fprintln(os.Stdout, "no matches")
			return nil
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SCORE\tSLUG\tCATEGORY\tNAME")
		for _, h := range hits {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", h.Score, h.Entry.Slug, h.Entry.Category, h.Entry.Name)
		}
		return tw.Flush()
	},
}

var registryShareCmd = &cobra.Command{
	Use:   "share <slug>",
	Short: "Open a pre-filled GitHub issue to upstream a local registry entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		// Full implementation lands in Step 12. Placeholder keeps the CLI surface stable.
		fmt.Printf("stackwright registry share: not yet implemented (slug=%q)\n", args[0])
		return nil
	},
}

func init() {
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registrySearchCmd)
	registryCmd.AddCommand(registryShareCmd)
}

func cliSummary(e registry.Entry) string {
	if e.CLI == nil {
		return "-"
	}
	return e.CLI.Binary
}

func authSummary(e registry.Entry) string {
	if e.Auth == nil || !e.Auth.Required {
		return "-"
	}
	return "yes"
}
