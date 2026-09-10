package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iamnikolie/slack-cli/internal/config"
	"github.com/spf13/cobra"
)

var initToken string

func maskToken(t string) string {
	if t == "" {
		return "not set"
	}
	if len(t) <= 4 {
		return "set"
	}
	return "set (…" + t[len(t)-4:] + ")"
}

// resolveInitToken picks the token: --token flag, else SLK_TOKEN env, else the
// first non-empty line from r. Errors if all are empty.
func resolveInitToken(flag string, r io.Reader) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env := os.Getenv("SLK_TOKEN"); env != "" {
		return env, nil
	}
	line, _ := bufio.NewReader(r).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("no token provided: pass --token, set SLK_TOKEN, or pipe it on stdin")
	}
	return line, nil
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage slk configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Save the Slack user token (writes ~/.slk/<profile>/config.yaml)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if initToken == "" && os.Getenv("SLK_TOKEN") == "" {
			fmt.Fprint(stderr, "Slack user token (xoxp-...): ")
		}
		tok, err := resolveInitToken(initToken, os.Stdin)
		if err != nil {
			return err
		}
		if err := config.Save(tok, profile); err != nil {
			return err
		}
		fmt.Printf("Saved to ~/.slk/%s/config.yaml\n", profile)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the active profile and token state",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := config.Load(profile)
		if err != nil {
			return err
		}
		row := map[string]any{
			"profile":  profile,
			"base_url": config.BaseURL,
			"token":    maskToken(c.Token),
		}
		b, _ := json.Marshal(row)
		return emitObj(b, []string{"profile", "base_url", "token"})
	},
}

func init() {
	configInitCmd.Flags().StringVar(&initToken, "token", "", "Slack user token (xoxp-...); else SLK_TOKEN or stdin")
	configCmd.AddCommand(configInitCmd, configShowCmd)
	rootCmd.AddCommand(configCmd)
}
