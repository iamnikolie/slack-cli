package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/iamnikolie/slack-cli/internal/client"
	"github.com/iamnikolie/slack-cli/internal/config"
	"github.com/spf13/cobra"
)

// version is stamped at link time by the Makefile and by GoReleaser
// (-X github.com/iamnikolie/slack-cli/cmd.version=…). An unstamped build says
// "dev" rather than claiming a version it does not have; buildVersion appends
// the VCS revision when the toolchain embedded one.
var version = "dev"

func buildVersion() string {
	v := version
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				rev := s.Value
				if len(rev) > 12 {
					rev = rev[:12]
				}
				v += " (" + rev + ")"
			}
		}
	}
	return v
}

var (
	jsonOutput   bool
	profile      string
	verbose      bool
	outputFormat string
	assumeYes    bool
	fieldsFlag   []string
	noLinks      bool
	maxChars     int

	cfg *config.Config
	cli *client.Client
	dir *cache.Directory // lazily loaded by loadDirectory()
)

var stderr io.Writer = os.Stderr

var rootCmd = &cobra.Command{
	Use:          "slk",
	Short:        "Slack CLI — agent-facing Slack from the terminal",
	Long:         "Slack CLI — agent-facing Slack from the terminal.\n\nRun 'slk skill' to print the full agent reference (commands, flags, workflows).",
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if profileExempt(cmd.Name()) {
			return nil
		}
		if profile == "" {
			return fmt.Errorf("--config <name> is required (or set SLK_CONFIG); there is no default profile — run 'slk --config <name> config init'")
		}
		// config init/show need the profile name but not a validated token.
		if cmd.Name() == "init" || cmd.Name() == "show" {
			return nil
		}
		var err error
		cfg, err = config.Load(profile)
		if err != nil {
			return err
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		cli = client.New(cfg.Token, config.BaseURL)
		cli.Verbose = verbose
		cli.Profile = profile
		return nil
	},
}

func Execute() {
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output UTF-8 JSON (all fields unless --fields is set)")
	rootCmd.PersistentFlags().StringVar(&profile, "config", os.Getenv("SLK_CONFIG"), "workspace profile (subdirectory of ~/.slk/)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "dump API request/response to stderr")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "format", "", "output format: table (default), json, csv, tsv")
	rootCmd.PersistentFlags().BoolVar(&assumeYes, "yes", false, "confirm destructive operations")
	rootCmd.PersistentFlags().StringSliceVar(&fieldsFlag, "fields", nil, "comma-separated output fields, including JSON (supports dotted object paths)")

	rootCmd.PersistentFlags().BoolVar(&noLinks, "no-links", false, "transcripts: omit per-message permalink lines (ts= still shown)")
	rootCmd.PersistentFlags().IntVar(&maxChars, "max-chars", 0, "transcripts: cap each message text at N characters (0 = no cap)")

	rootCmd.Version = buildVersion()
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the slk version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("slk version %s\n", buildVersion())
		return nil
	},
}

// profileExempt reports whether a command runs without a profile or token.
func profileExempt(name string) bool {
	switch name {
	case "slk", "skill", "help", "completion", "version", "bash", "zsh", "fish", "powershell":
		return true
	}
	return false
}

func paginationHint(w io.Writer, hitLimit bool, limit int) {
	if hitLimit {
		fmt.Fprintf(w, "(showing %d results — limit reached; pass --limit %d for more)\n", limit, limit*2)
	}
}
