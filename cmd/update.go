package cmd

import (
	"net/url"

	"github.com/spf13/cobra"
)

var updateBodyFile string

var updateCmd = &cobra.Command{
	Use:   "update <#channel|id> <ts> [text]",
	Short: "Edit your own message (chat.update)",
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
		body, err := readBody(text, updateBodyFile)
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("channel", id)
		q.Set("ts", ts)
		q.Set("text", body)
		raw, err := cli.Call(cmd.Context(), "chat.update", q)
		if err != nil {
			return err
		}
		return emitObj(raw, []string{"ok", "channel", "ts"})
	},
}

func init() {
	updateCmd.Flags().StringVar(&updateBodyFile, "body-file", "", "read text from a file ('-' for stdin)")
	rootCmd.AddCommand(updateCmd)
}
