package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

var (
	historyLimit        int
	historyOldest       string
	historyLatest       string
	historySince        string
	historyUntil        string
	historyThread       string
	historyReplies      bool
	historyRepliesLimit int
	historyDesc         bool
)

var historyCmd = &cobra.Command{
	Use:   "history <#channel|id|permalink>",
	Short: "Read channel message history (transcript), optionally with thread replies",
	Long: `Read channel message history (transcript).

--limit takes the latest N messages in the window; they print oldest first
(--desc for newest first). --since/--until take 2h/3d/1w, today, yesterday,
YYYY-MM-DD[ HH:MM] (local), RFC3339, epoch, or a ts.

  --thread <ts|permalink>  read one thread instead of the channel (same as 'slk thread')
  --replies                expand every thread inline under its parent message

A thread permalink as the only argument implies --thread.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ref := args[0]
		if ch, _, ok := parseThreadRef(ref); ok && ch != "" && historyThread == "" {
			historyThread, ref = ref, ch
		}
		bounds, err := timeBounds(historyOldest, historyLatest, historySince, historyUntil)
		if err != nil {
			return err
		}
		if historyThread != "" {
			if historyReplies {
				return fmt.Errorf("--thread and --replies are exclusive: --thread already reads the replies")
			}
			channel, ts, err := resolveThreadTarget(ctx, []string{ref, historyThread})
			if err != nil {
				return err
			}
			limit := historyLimit
			if !cmd.Flags().Changed("limit") {
				limit = threadLimit
			}
			return readThread(ctx, channel, ts, limit, bounds)
		}

		id, err := channelRef(ctx, ref)
		if err != nil {
			return err
		}
		q := bounds
		q.Set("channel", id)
		raw, hit, err := cli.Paginate(ctx, "conversations.history", q, "messages", historyLimit)
		if err != nil {
			return err
		}
		paginationHint(stderr, hit, historyLimit)
		if !historyDesc {
			raw = reverseArray(raw)
		}

		if historyReplies {
			if raw, err = expandReplies(ctx, raw, id, historyRepliesLimit, ""); err != nil {
				return err
			}
		}
		return emitTranscript(ctx, raw, id, "")
	},
}

// timeBounds builds Slack's oldest/latest from the raw ts flags and the
// friendlier --since/--until (which win when both are given).
func timeBounds(oldest, latest, since, until string) (url.Values, error) {
	q := url.Values{}
	if oldest != "" {
		q.Set("oldest", oldest)
	}
	if latest != "" {
		q.Set("latest", latest)
	}
	for key, v := range map[string]string{"oldest": since, "latest": until} {
		if v == "" {
			continue
		}
		bound, err := parseTimeBound(v)
		if err != nil {
			return nil, err
		}
		q.Set(key, bound)
	}
	return q, nil
}

// reverseArray flips a JSON array; Slack returns history newest first.
func reverseArray(raw json.RawMessage) json.RawMessage {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return raw
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	b, err := json.Marshal(items)
	if err != nil {
		return raw
	}
	return b
}

// expandReplies attaches each thread parent's replies (oldest first, parent
// excluded) under a "replies" key; only, when set, limits it to that parent. A
// thread that fails to load keeps its reply count and is reported on stderr
// instead of failing the whole history.
func expandReplies(ctx context.Context, raw json.RawMessage, channel string, perThread int, only string) (json.RawMessage, error) {
	msgs, err := decodeArray(raw)
	if err != nil {
		return nil, fmt.Errorf("expandReplies: %w", err)
	}
	for _, m := range msgs {
		ts, _ := m["ts"].(string)
		count, _ := m["reply_count"].(json.Number)
		if n, _ := count.Int64(); n <= 0 || ts == "" || (only != "" && ts != only) {
			continue
		}
		if parent, _ := m["thread_ts"].(string); parent != "" && parent != ts {
			continue // a broadcast reply, not a thread parent
		}
		// +1: conversations.replies returns the parent first.
		thread, _, err := fetchReplies(ctx, channel, ts, perThread+1, nil)
		if err != nil {
			fmt.Fprintf(stderr, "thread %s: replies unavailable: %v\n", ts, err)
			continue
		}
		items, err := decodeArray(thread)
		if err != nil {
			return nil, fmt.Errorf("expandReplies: %w", err)
		}
		replies := make([]map[string]any, 0, len(items))
		for _, r := range items {
			if r["ts"] != ts {
				replies = append(replies, r)
			}
		}
		m["replies"] = replies
	}
	return json.Marshal(msgs)
}

// flattenReplies lifts expanded replies into the top-level list right after
// their parent, so table/CSV output gets one row per message.
func flattenReplies(raw json.RawMessage) json.RawMessage {
	msgs, err := decodeArray(raw)
	if err != nil {
		return raw
	}
	out := make([]map[string]any, 0, len(msgs))
	expanded := false
	for _, m := range msgs {
		replies, ok := m["replies"].([]any)
		delete(m, "replies")
		out = append(out, m)
		if !ok {
			continue
		}
		expanded = true
		for _, r := range replies {
			if rm, ok := r.(map[string]any); ok {
				out = append(out, rm)
			}
		}
	}
	if !expanded {
		return raw
	}
	b, err := json.Marshal(out)
	if err != nil {
		return raw
	}
	return b
}

func init() {
	historyCmd.Flags().IntVar(&historyLimit, "limit", 50, "max messages (with --thread: max thread messages, default 200)")
	historyCmd.Flags().StringVar(&historyOldest, "oldest", "", "only messages after this ts (epoch or ts)")
	historyCmd.Flags().StringVar(&historyLatest, "latest", "", "only messages before this ts (epoch or ts)")
	historyCmd.Flags().StringVar(&historySince, "since", "", "only messages on/after this time (2h, 3d, 1w, today, yesterday, YYYY-MM-DD[ HH:MM], epoch, ts)")
	historyCmd.Flags().StringVar(&historyUntil, "until", "", "only messages before this time (same forms as --since)")
	historyCmd.Flags().StringVar(&historyThread, "thread", "", "read this thread instead of the channel (parent ts or permalink)")
	historyCmd.Flags().BoolVar(&historyReplies, "replies", false, "expand thread replies inline under each parent (one API call per thread)")
	historyCmd.Flags().IntVar(&historyRepliesLimit, "replies-limit", 50, "max replies shown per thread with --replies")
	historyCmd.Flags().BoolVar(&historyDesc, "desc", false, "newest message first (default: oldest first)")
	rootCmd.AddCommand(historyCmd)
}
