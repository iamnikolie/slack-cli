package cmd

import (
	"context"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

var threadLimit int

var threadCmd = &cobra.Command{
	Use:   "thread <#channel|id> <ts|permalink>",
	Short: "Read all replies in a thread (comments)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		channel, ts, err := resolveThreadTarget(cmd.Context(), args)
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("channel", channel)
		q.Set("ts", ts)
		raw, hit, err := cli.Paginate(cmd.Context(), "conversations.replies", q, "messages", threadLimit)
		if err != nil {
			return err
		}
		paginationHint(stderr, hit, threadLimit)

		return emitTranscript(cmd.Context(), raw, channel, ts)
	},
}

// resolveThreadTarget accepts either (permalink) or (#channel, ts). A permalink
// carries its own channel; a bare ts requires the channel arg.
func resolveThreadTarget(ctx context.Context, args []string) (channel, ts string, err error) {
	if len(args) == 1 {
		ch, t, ok := parseMessageRef(args[0])
		if !ok || ch == "" {
			return "", "", fmt.Errorf("with one argument, pass a Slack permalink; otherwise give <#channel> <ts>")
		}
		return ch, t, nil
	}
	ch, err := channelRef(ctx, args[0])
	if err != nil {
		return "", "", err
	}
	_, t, ok := parseMessageRef(args[1])
	if !ok {
		return "", "", fmt.Errorf("invalid ts %q (expected 1700000000.000100 or a permalink)", args[1])
	}
	return ch, t, nil
}

func init() {
	threadCmd.Flags().IntVar(&threadLimit, "limit", 200, "max replies")
	rootCmd.AddCommand(threadCmd)
}
