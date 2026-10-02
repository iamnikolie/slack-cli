package cmd

import (
	"github.com/spf13/cobra"
)

var replyFlags msgFlags

var replyCmd = &cobra.Command{
	Use:   "reply <#channel|@user|id> <ts|permalink> [text]",
	Short: "Reply in a thread (sugar over `send --thread`)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := channelRef(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		_, ts, ok := parseThreadRef(args[1])
		if !ok {
			return errInvalidTS(args[1])
		}
		text := ""
		if len(args) == 3 {
			text = args[2]
		}
		return runMessage(cmd.Context(), id, ts, text, &replyFlags)
	},
}

func init() {
	replyFlags.register(replyCmd, true)
	rootCmd.AddCommand(replyCmd)
}
