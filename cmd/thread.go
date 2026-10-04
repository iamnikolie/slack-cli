package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	threadLimit int
	threadSince string
	threadUntil string
)

var threadCmd = &cobra.Command{
	Use:   "thread <#channel|id> <ts|permalink>",
	Short: "Read all replies in a thread (comments)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		channel, ts, err := resolveThreadTarget(cmd.Context(), args)
		if err != nil {
			return err
		}
		bounds, err := timeBounds("", "", threadSince, threadUntil)
		if err != nil {
			return err
		}
		return readThread(cmd.Context(), channel, ts, threadLimit, bounds)
	},
}

// readThread prints a thread (parent first, then replies) as a transcript.
// bounds may carry oldest/latest to window a long thread.
func readThread(ctx context.Context, channel, ts string, limit int, bounds url.Values) error {
	raw, hit, err := fetchReplies(ctx, channel, ts, limit, bounds)
	if err != nil {
		return err
	}
	paginationHint(stderr, hit, limit)
	if err := emitTranscript(ctx, raw, channel, ts); err != nil {
		return err
	}
	if len(bounds) > 0 && !jsonOutput && outputFormat == "" && len(fieldsFlag) == 0 {
		var msgs []transcriptMsg
		if json.Unmarshal(raw, &msgs) == nil && len(msgs) == 1 && msgs[0].ReplyCount > 0 {
			fmt.Printf("  ↳ 0 of %d %s in the window\n", msgs[0].ReplyCount, plural(msgs[0].ReplyCount, "reply", "replies"))
		}
	}
	return nil
}

// fetchReplies returns a thread's parent, then its latest limit replies oldest
// first; more reports replies left out. Slack pages a thread from the newest
// replies back and repeats the parent on every page, so pages are merged by ts
// rather than concatenated. limit 0 returns just the message at ts.
func fetchReplies(ctx context.Context, channel, ts string, limit int, bounds url.Values) (raw json.RawMessage, more bool, err error) {
	q := url.Values{}
	for k, v := range bounds {
		q[k] = v
	}
	q = pinLatest(q)
	q.Set("channel", channel)
	q.Set("ts", ts)

	var parent json.RawMessage
	var replies []json.RawMessage
	seen := map[string]bool{}
	cursor := ""
	for {
		q.Set("limit", strconv.Itoa(min(max(limit-len(replies), 1), 200)))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		b, err := cli.Call(ctx, "conversations.replies", q)
		if err != nil {
			return nil, false, err
		}
		var page struct {
			Messages []json.RawMessage `json:"messages"`
			HasMore  bool              `json:"has_more"`
			Meta     struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := json.Unmarshal(b, &page); err != nil {
			return nil, false, fmt.Errorf("fetchReplies: decode: %w", err)
		}
		for _, m := range page.Messages {
			var h struct {
				TS string `json:"ts"`
			}
			_ = json.Unmarshal(m, &h)
			switch {
			case h.TS == ts:
				if parent == nil {
					parent = m
				}
			case !seen[h.TS]:
				seen[h.TS] = true
				replies = append(replies, m)
			}
		}
		cursor = page.Meta.NextCursor
		if cursor == "" || len(replies) >= limit {
			more = cursor != "" || page.HasMore
			break
		}
	}
	sortRawByTS(replies)
	if len(replies) > limit {
		more = true
		replies = replies[len(replies)-limit:]
	}
	out := make([]json.RawMessage, 0, len(replies)+1)
	if parent != nil {
		out = append(out, parent)
	}
	raw, err = json.Marshal(append(out, replies...))
	return raw, more, err
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
	threadCmd.Flags().IntVar(&threadLimit, "limit", 200, "max replies (the latest N)")
	threadCmd.Flags().StringVar(&threadSince, "since", "", "only replies on/after this time (2h, 3d, today, YYYY-MM-DD, …)")
	threadCmd.Flags().StringVar(&threadUntil, "until", "", "only replies before this time")
	rootCmd.AddCommand(threadCmd)
}
