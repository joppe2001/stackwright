package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"

	"github.com/joppe2001/stackwright/internal/browser"
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
	Long: `Reads <slug> from registry.local.yaml and opens a GitHub issue on
joppe2001/stackwright-registry with the entry body pre-filled. Use this to
contribute a tool you've added locally to the community catalog.`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return shareLocalEntry(args[0])
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

// shareLocalEntry reads the named slug from registry.local.yaml, formats the
// entry as a YAML block, URL-encodes it as a GitHub issue body, and opens
// the user's browser at the issue-new page on the registry repo.
//
// No network call is made — we just construct the URL and shell out to the
// OS "open URL" helper. The user reviews and submits the issue themselves.
const (
	registryOwner = "joppe2001"
	registryRepo  = "stackwright-registry"
)

func shareLocalEntry(slug string) error {
	local, err := registry.ReadLocal()
	if err != nil {
		return fmt.Errorf("read local registry: %w", err)
	}
	if local == nil {
		return fmt.Errorf("no registry.local.yaml found — run stackwright and add a tech via 'a' first")
	}
	var entry *registry.Entry
	for i := range local.Entries {
		if local.Entries[i].Slug == slug {
			entry = &local.Entries[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("slug %q not found in registry.local.yaml", slug)
	}

	body, err := formatIssueBody(*entry)
	if err != nil {
		return err
	}

	title := fmt.Sprintf("Add %s", entry.Name)
	issueURL := fmt.Sprintf("https://github.com/%s/%s/issues/new?title=%s&body=%s",
		registryOwner, registryRepo,
		url.QueryEscape(title),
		url.QueryEscape(body),
	)

	if err := browser.Open(issueURL); err != nil {
		// Print the URL so the user can open it manually if every opener failed.
		fmt.Fprintln(os.Stdout, "could not open browser:", err)
		fmt.Fprintln(os.Stdout, "visit this URL manually:")
		fmt.Fprintln(os.Stdout, issueURL)
		return nil
	}
	fmt.Fprintf(os.Stdout, "Opened browser with pre-filled issue for %s.\nThank you for contributing.\n", entry.Name)
	return nil
}

// formatIssueBody renders a YAML entry plus a brief contribution note.
// Kept simple: a markdown wrapper around the YAML makes the issue readable
// both in the browser and after maintainers accept it.
func formatIssueBody(e registry.Entry) (string, error) {
	var buf strings.Builder
	buf.WriteString("Contributing a new technology to the stackwright registry.\n\n")
	buf.WriteString("## Proposed entry\n\n")
	buf.WriteString("```yaml\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(e); err != nil {
		return "", err
	}
	_ = enc.Close()
	buf.WriteString("```\n\n")
	buf.WriteString("## Why\n\n")
	buf.WriteString("_Explain briefly why this tool belongs in the catalog._\n")
	return buf.String(), nil
}

// (openInBrowser removed — the shared internal/browser package handles
// all platform strategies plus WSL and $BROWSER fallbacks.)
