package cmd

import (
	"github.com/spf13/cobra"
)

var (
	replyBodyFile string
	replyIDOnly   bool
)

var replyCmd = &cobra.Command{
	Use:   "reply <#channel|id> <ts> [text]",
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
		body, err := readBody(text, replyBodyFile)
		if err != nil {
			return err
		}
		raw, err := postMessage(cmd, id, body, ts)
		if err != nil {
			return err
		}
		if replyIDOnly {
			return printIDOnly(raw, "ts")
		}
		return emitObj(raw, []string{"ok", "channel", "ts"})
	},
}

func init() {
	replyCmd.Flags().StringVar(&replyBodyFile, "body-file", "", "read text from a file ('-' for stdin)")
	replyCmd.Flags().BoolVar(&replyIDOnly, "id-only", false, "print only the new message ts")
	rootCmd.AddCommand(replyCmd)
}
