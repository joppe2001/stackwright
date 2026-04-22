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
	FlagKitty   bool // opt IN to the Kitty GFX renderer
	FlagOffline bool
)

var (
	flagDetect  bool
	flagVersion bool
	flagNoKitty bool // back-compat alias; kept so --no-kitty doesn't error out.
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
		// useVisual: true only if the user explicitly opted in AND didn't also
		// pass --no-kitty (which wins for back-compat).
		useVisual := FlagKitty && !flagNoKitty
		if flagDetect {
			caps := detect.Probe()
			detect.PrintReport(os.Stdout, caps, !useVisual)
			return nil
		}
		return runTUI(!useVisual, FlagOffline)
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&FlagKitty, "kitty", false, "opt in to the Kitty Graphics renderer (experimental — some artifacts on resize)")
	rootCmd.PersistentFlags().BoolVar(&flagNoKitty, "no-kitty", false, "back-compat: force standard mode (now the default)")
	rootCmd.PersistentFlags().BoolVar(&FlagOffline, "offline", false, "skip registry fetch; use bundled + local only")
	rootCmd.Flags().BoolVar(&flagDetect, "detect", false, "print terminal capability report and exit")
	rootCmd.Flags().BoolVar(&flagVersion, "version", false, "print version and exit")

	rootCmd.AddCommand(registryCmd)
}

// Execute runs the root cobra command.
func Execute() error {
	return rootCmd.Execute()
}
