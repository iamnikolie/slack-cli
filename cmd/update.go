package cmd

import (
	"github.com/spf13/cobra"
)

var updateFlags msgFlags

var updateCmd = &cobra.Command{
	Use:   "update <#channel|@user|id> <ts> [text]",
	Short: "Edit your own message (chat.update; Markdown by default)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := channelRef(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		_, ts, ok := parseMessageRef(args[1])
		if !ok {
			return errInvalidTS(args[1])
		}
		text := ""
		if len(args) == 3 {
			text = args[2]
		}
		body, err := readBody(text, updateFlags.bodyFile)
		if err != nil {
			return err
		}
		raw, err := deliver(cmd.Context(), outgoing{Channel: id, TS: ts, Text: body}, &updateFlags)
		if err != nil {
			return err
		}
		return emitDelivered(raw, &updateFlags)
	},
}

func init() {
	updateFlags.register(updateCmd, false)
	rootCmd.AddCommand(updateCmd)
}
