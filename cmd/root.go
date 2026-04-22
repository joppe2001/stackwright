package cmd

import (
	"fmt"
	"os"

	"github.com/joppe2001/stackwright/internal/detect"
	"github.com/spf13/cobra"
)

// Version is stamped at build time via -ldflags "-X github.com/joppe2001/stackwright/cmd.Version=<v>".
var Version = "0.1.0-dev"

// Global flags. Exported so subcommands can read them.
var (
	FlagNoKitty bool
	FlagOffline bool
)

var (
	flagDetect  bool
	flagVersion bool
)

var rootCmd = &cobra.Command{
	Use:   "stackwright",
	Short: "Interactive full-stack project architecture builder",
	Long: `stackwright composes a full-stack project stack in an interactive TUI,
walks through CLI install and auth for every selected technology,
then scaffolds a complete project with boilerplate, stack.yaml, SETUP.md,
and an architecture diagram PNG.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagVersion {
			fmt.Println("stackwright", Version)
			return nil
		}
		if flagDetect {
			caps := detect.Probe()
			detect.PrintReport(os.Stdout, caps, FlagNoKitty)
			return nil
		}
		// Default action: launch the TUI run flow.
		return runTUI(FlagNoKitty, FlagOffline)
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&FlagNoKitty, "no-kitty", false, "force standard (ANSI/Unicode) render mode")
	rootCmd.PersistentFlags().BoolVar(&FlagOffline, "offline", false, "skip registry fetch; use bundled + local only")
	rootCmd.Flags().BoolVar(&flagDetect, "detect", false, "print terminal capability report and exit")
	rootCmd.Flags().BoolVar(&flagVersion, "version", false, "print version and exit")

	rootCmd.AddCommand(registryCmd)
}

// Execute runs the root cobra command.
func Execute() error {
	return rootCmd.Execute()
}
