package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <#channel|id> <ts>",
	Short: "Delete your own message (chat.delete); requires --yes",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !assumeYes {
			return fmt.Errorf("refusing to delete without --yes")
		}
		id, err := channelRef(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		_, ts, ok := parseMessageRef(args[1])
		if !ok {
			return errInvalidTS(args[1])
		}
		q := url.Values{}
		q.Set("channel", id)
		q.Set("ts", ts)
		raw, err := cli.Call(cmd.Context(), "chat.delete", q)
		if err != nil {
			return err
		}
		return emitObj(raw, []string{"ok", "channel", "ts"})
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
