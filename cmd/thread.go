package cmd

import (
	"context"
	"encoding/json"
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
		return readThread(cmd.Context(), channel, ts, threadLimit)
	},
}

// readThread prints a thread (parent first, then replies) as a transcript.
func readThread(ctx context.Context, channel, ts string, limit int) error {
	raw, hit, err := fetchReplies(ctx, channel, ts, limit)
	if err != nil {
		return err
	}
	paginationHint(stderr, hit, limit)
	return emitTranscript(ctx, raw, channel, ts)
}

// fetchReplies returns up to limit messages of a thread; the parent comes first.
func fetchReplies(ctx context.Context, channel, ts string, limit int) (json.RawMessage, bool, error) {
	q := url.Values{}
	q.Set("channel", channel)
	q.Set("ts", ts)
	return cli.Paginate(ctx, "conversations.replies", q, "messages", limit)
}

// resolveThreadTarget accepts either (permalink) or (#channel, ts). A permalink
// carries its own channel; a bare ts requires the channel arg.
func resolveThreadTarget(ctx context.Context, args []string) (channel, ts string, err error) {
	if len(args) == 1 {
		ch, t, ok := parseThreadRef(args[0])
		if !ok || ch == "" {
			return "", "", fmt.Errorf("with one argument, pass a Slack permalink; otherwise give <#channel> <ts>")
		}
		return ch, t, nil
	}
	ch, err := channelRef(ctx, args[0])
	if err != nil {
		return "", "", err
	}
	_, t, ok := parseThreadRef(args[1])
	if !ok {
		return "", "", fmt.Errorf("invalid ts %q (expected 1700000000.000100 or a permalink)", args[1])
	}
	return ch, t, nil
}

func init() {
	threadCmd.Flags().IntVar(&threadLimit, "limit", 200, "max replies")
	rootCmd.AddCommand(threadCmd)
}
