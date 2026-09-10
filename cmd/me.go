package cmd

import (
	"github.com/spf13/cobra"
)

var meCmd = &cobra.Command{
	Use:   "me",
	Short: "Show the authenticated identity (auth.test)",
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := cli.Call(cmd.Context(), "auth.test", nil)
		if err != nil {
			return err
		}
		return emitObj(raw, []string{"user", "user_id", "team", "team_id", "url"})
	},
}

func init() {
	rootCmd.AddCommand(meCmd)
}
