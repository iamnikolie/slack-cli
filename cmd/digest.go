package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/spf13/cobra"
)

// section is one channel's slice of a multi-channel transcript.
type section struct {
	Channel  string          `json:"channel"`
	Name     string          `json:"name"`
	Messages json.RawMessage `json:"messages"`
}

// emitSections prints one transcript per non-empty channel, or a JSON array
// of {channel,name,messages}. empty is what to say when nothing matched.
func emitSections(ctx context.Context, sections []section, empty string) error {
	ids := make([]string, len(sections))
	for i, s := range sections {
		ids[i] = s.Channel
	}
	d, err := directoryFor(ctx, ids...)
	if err != nil {
		return err
	}
	var kept []section
	for _, s := range sections {
		var msgs []json.RawMessage
		if json.Unmarshal(s.Messages, &msgs) == nil && len(msgs) > 0 {
			s.Name = channelLabel(d, s.Channel)
			kept = append(kept, s)
		}
	}
	if jsonOutput || outputFormat == "json" {
		if kept == nil {
			kept = []section{}
		}
		for i := range kept {
			kept[i].Messages = projectList(kept[i].Messages, fieldsFlag)
		}
		b, err := json.Marshal(kept)
		if err != nil {
			return err
		}
		return writeRaw(b)
	}
	if len(kept) == 0 {
		fmt.Println(empty)
		return nil
	}
	baseURL := ""
	if !noLinks {
		baseURL = workspaceURL(ctx)
	}
	for i, s := range kept {
		if i > 0 {
			fmt.Println()
		}
		out, err := renderTranscript(s.Messages, d, s.Channel, baseURL, "")
		if err != nil {
			return err
		}
		if _, err := os.Stdout.WriteString(out); err != nil {
			return err
		}
	}
	return nil
}

// channelWindow returns up to limit messages after oldest (and before until,
// when set), oldest first.
// With only oldest set Slack returns the messages right after it (the earliest
// of the window, which tail and get's after-context want).
func channelWindow(ctx context.Context, channel, oldest, until string, limit int) (json.RawMessage, bool, error) {
	q := url.Values{}
	q.Set("channel", channel)
	q.Set("oldest", oldest)
	// No latest: Slack would then page from until backwards and a capped read
	// would keep the last N, not the first. Messages past until are cut here.
	raw, hit, err := cli.Paginate(ctx, "conversations.history", q, "messages", limit)
	if err != nil {
		return nil, false, err
	}
	raw = sortByTS(raw, false)
	if until == "" {
		return raw, hit, nil
	}
	var msgs []json.RawMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, false, fmt.Errorf("channelWindow: %w", err)
	}
	kept := msgs[:0]
	for _, m := range msgs {
		var h struct {
			TS string `json:"ts"`
		}
		if json.Unmarshal(m, &h) == nil && h.TS < until {
			kept = append(kept, m)
		}
	}
	if len(kept) < len(msgs) {
		hit = false // the read reached past the window: nothing in it was left out
	}
	raw, err = json.Marshal(kept)
	return raw, hit, err
}

// activeChannels finds channels with indexed messages in [since, until) via
// search (zero until = no end), most recent first. Search is a sample:
// channels it does not index stay invisible.
func activeChannels(ctx context.Context, since, until time.Time) ([]string, error) {
	// after:/before: are exclusive by day, so widen by a day and filter by ts below.
	query := "after:" + since.AddDate(0, 0, -1).Format("2006-01-02")
	untilTS := ""
	if !until.IsZero() {
		query += " before:" + until.AddDate(0, 0, 1).Format("2006-01-02")
		untilTS = tsOf(until)
	}
	result, err := fetchSearch(ctx, query, 1000, "desc")
	if err != nil {
		return nil, err
	}
	sinceTS := tsOf(since)
	seen := map[string]bool{}
	var ids []string
	for _, raw := range result.Matches {
		var m struct {
			TS      string `json:"ts"`
			Channel struct {
				ID string `json:"id"`
			} `json:"channel"`
		}
		if json.Unmarshal(raw, &m) != nil || m.Channel.ID == "" || m.TS < sinceTS || (untilTS != "" && m.TS >= untilTS) || seen[m.Channel.ID] {
			continue
		}
		seen[m.Channel.ID] = true
		ids = append(ids, m.Channel.ID)
	}
	if result.SearchMeta.HasMore {
		fmt.Fprintf(stderr, "channel discovery sampled %d of %d indexed messages; pass channels explicitly to be exhaustive\n", result.SearchMeta.Returned, result.SearchMeta.Total)
	}
	return ids, nil
}

// channelArgs resolves explicit channel args, or discovers active ones.
func channelArgs(ctx context.Context, args []string, since, until time.Time) ([]string, error) {
	if len(args) == 0 {
		return activeChannels(ctx, since, until)
	}
	ids := make([]string, 0, len(args))
	for _, a := range args {
		for _, ref := range strings.Split(a, ",") {
			if ref = strings.TrimSpace(ref); ref == "" {
				continue
			}
			id, err := channelRef(ctx, ref)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", ref, err)
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// ---- get -------------------------------------------------------------------

var (
	getContext      int
	getRepliesLimit int
)

var getCmd = &cobra.Command{
	Use:   "get <permalink | #channel ts>",
	Short: "Read one message with surrounding context (and its thread)",
	Long: `Read one message, marked with », plus --context N messages before and after.

A top-level message shows its thread replies inline (--replies-limit). A reply
shows its thread: the parent plus N replies around it.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		channel, ts, thread, err := resolveMessageTarget(ctx, args)
		if err != nil {
			return err
		}
		if thread == "" {
			// conversations.replies on any ts returns that message, with
			// thread_ts when it is a reply.
			raw, _, err := fetchReplies(ctx, channel, ts, 1, nil)
			if err != nil {
				return err
			}
			msgs, err := decodeArray(raw)
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				return fmt.Errorf("no message at %s in %s", ts, channel)
			}
			thread, _ = msgs[0]["thread_ts"].(string)
		}
		if thread != "" && thread != ts {
			raw, _, err := fetchReplies(ctx, channel, thread, 1000, nil)
			if err != nil {
				return err
			}
			window, err := focusWindow(raw, ts, getContext, true)
			if err != nil {
				return err
			}
			return emitTranscript(ctx, window, channel, thread)
		}

		q := url.Values{"channel": {channel}, "latest": {ts}, "inclusive": {"true"}}
		before, _, err := cli.Paginate(ctx, "conversations.history", q, "messages", getContext+1)
		if err != nil {
			return err
		}
		msgs := sortByTS(before, false)
		if getContext > 0 {
			after, _, err := channelWindow(ctx, channel, ts, "", getContext)
			if err != nil {
				return err
			}
			msgs = concatArrays(msgs, after)
		}
		window, err := focusWindow(msgs, ts, getContext, false)
		if err != nil {
			return err
		}
		if window, err = expandReplies(ctx, window, channel, getRepliesLimit, ts); err != nil {
			return err
		}
		return emitTranscript(ctx, window, channel, "")
	},
}

// resolveMessageTarget accepts (permalink) or (#channel, ts|permalink) and
// also returns the parent ts a reply's permalink carries.
func resolveMessageTarget(ctx context.Context, args []string) (channel, ts, thread string, err error) {
	ref := args[len(args)-1]
	ch, t, ok := parseMessageRef(ref)
	if !ok {
		return "", "", "", errInvalidTS(ref)
	}
	if m := rePermalinkThread.FindStringSubmatch(ref); m != nil && ch != "" {
		thread = m[1]
	}
	if len(args) == 2 {
		if ch, err = channelRef(ctx, args[0]); err != nil {
			return "", "", "", err
		}
	}
	if ch == "" {
		return "", "", "", fmt.Errorf("with one argument, pass a Slack permalink; otherwise give <#channel> <ts>")
	}
	return ch, t, thread, nil
}

// focusWindow keeps context messages on each side of ts and marks it. With
// keepFirst the first message (a thread parent) always stays.
func focusWindow(raw json.RawMessage, ts string, context int, keepFirst bool) (json.RawMessage, error) {
	msgs, err := decodeArray(raw)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, m := range msgs {
		if m["ts"] == ts {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("message %s not found (deleted, or not visible to this token)", ts)
	}
	msgs[idx]["slk_focus"] = true
	lo, hi := max(0, idx-context), min(len(msgs)-1, idx+context)
	out := []map[string]any{}
	if keepFirst && lo > 0 {
		out = append(out, msgs[0])
	}
	out = append(out, msgs[lo:hi+1]...)
	return json.Marshal(out)
}

func concatArrays(a, b json.RawMessage) json.RawMessage {
	var x, y []json.RawMessage
	_ = json.Unmarshal(a, &x)
	_ = json.Unmarshal(b, &y)
	out, _ := json.Marshal(append(x, y...))
	return out
}

// ---- digest / tail -----------------------------------------------------------

var (
	digestSince        string
	digestUntil        string
	digestLimit        int
	digestReplies      bool
	digestRepliesLimit int
	tailPeek           bool
)

var digestCmd = &cobra.Command{
	Use:   "digest [#channel...]",
	Short: "One transcript across channels since a time, threads expanded",
	Long: `Transcripts of several channels since --since (default 1d), oldest first,
with thread replies inline. --until ends the window (same forms as --since);
--limit keeps the first N messages of it per channel, and a thread shows its
replies whenever they came. Without channels, active ones are discovered via
search (a sample; pass channels to be exhaustive). New replies under parents
older than --since do not appear: use 'slk unread' for those.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		now := time.Now()
		since, err := parsePast(digestSince, now)
		if err != nil {
			return err
		}
		var until time.Time
		untilTS, empty := "", "No messages since "+since.Format("2006-01-02 15:04 -07:00")+"."
		if digestUntil != "" {
			if until, err = parsePast(digestUntil, now); err != nil {
				return err
			}
			if !until.After(since) {
				return fmt.Errorf("--until (%s) is not after --since (%s): the window is empty", until.Format("2006-01-02 15:04:05 -07:00"), since.Format("2006-01-02 15:04:05 -07:00"))
			}
			untilTS = tsOf(until)
			empty = "No messages between " + since.Format("2006-01-02 15:04 -07:00") + " and " + until.Format("2006-01-02 15:04 -07:00") + "."
		}
		ids, err := channelArgs(ctx, args, since, until)
		if err != nil {
			return err
		}
		sections, err := channelSections(ctx, ids, func(string) string { return tsOf(since) }, untilTS, nil)
		if err != nil {
			return err
		}
		return emitSections(ctx, sections, empty)
	},
}

var tailCmd = &cobra.Command{
	Use:   "tail <#channel...>",
	Short: "Only what is new in channels since the last tail (cursor per channel)",
	Long: `Print messages newer than the last 'slk tail' of each channel, then move the
cursor (~/.slk/<profile>/cursors.json). A channel's first tail starts at
--since (default 1d). An explicit --since also caps how far back a later tail
reads (it starts at the later of cursor and --since), so a loop passing it
never repeats a message. --peek reads without moving cursors.

The cursor follows top-level messages: a new reply in an older thread is not
shown here — 'slk unread' covers threads you follow.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		since, err := parsePast(digestSince, time.Now())
		if err != nil {
			return err
		}
		ids, err := channelArgs(ctx, args, since, time.Time{})
		if err != nil {
			return err
		}
		cursors, err := cache.LoadCursors(profile)
		if err != nil {
			return err
		}
		oldest := func(id string) string {
			floor := tsOf(since)
			c := cursors[id]
			if c == "" || (cmd.Flags().Changed("since") && floor > c) {
				return floor
			}
			return c
		}
		moved := cache.Cursors{}
		sections, err := channelSections(ctx, ids, oldest, "", moved)
		if err != nil {
			return err
		}
		if err := emitSections(ctx, sections, "Nothing new."); err != nil {
			return err
		}
		if tailPeek || len(moved) == 0 {
			return nil
		}
		for id, ts := range moved {
			cursors[id] = ts
		}
		return cache.SaveCursors(cursors, profile)
	},
}

// channelSections reads each channel from oldest(id) up to until ("" = now);
// newest, when non-nil, collects the latest ts seen per channel.
func channelSections(ctx context.Context, ids []string, oldest func(string) string, until string, newest cache.Cursors) ([]section, error) {
	var sections []section
	for _, id := range ids {
		raw, hit, err := channelWindow(ctx, id, oldest(id), until, digestLimit)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if hit {
			fmt.Fprintf(stderr, "%s: showing the first %d %s; pass --limit %d for more\n", id, digestLimit, plural(digestLimit, "message", "messages"), digestLimit*2)
		}
		if newest != nil {
			var msgs []struct {
				TS string `json:"ts"`
			}
			_ = json.Unmarshal(raw, &msgs)
			if n := len(msgs); n > 0 {
				newest[id] = msgs[n-1].TS
			}
		}
		if digestReplies {
			if raw, err = expandReplies(ctx, raw, id, digestRepliesLimit, ""); err != nil {
				return nil, err
			}
		}
		sections = append(sections, section{Channel: id, Messages: raw})
	}
	return sections, nil
}

// ---- unread ------------------------------------------------------------------

var (
	unreadSince string
	unreadLimit int
)

var unreadCmd = &cobra.Command{
	Use:   "unread [#channel...]",
	Short: "Unread messages and unread replies in threads you follow",
	Long: `Per channel: messages after your read marker, plus followed threads with
replies you have not read (only new replies are shown). Looks at activity since
--since (default 7d); without channels, active ones are discovered via search.
Your own messages are skipped. Reading here does not mark anything read.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		since, err := parsePast(unreadSince, time.Now())
		if err != nil {
			return err
		}
		me, err := myUserID(ctx)
		if err != nil {
			return err
		}
		ids, err := channelArgs(ctx, args, since, time.Time{})
		if err != nil {
			return err
		}
		var sections []section
		for _, id := range ids {
			msgs, err := unreadIn(ctx, id, me, tsOf(since))
			if err != nil {
				fmt.Fprintf(stderr, "%s: skipped: %v\n", id, err)
				continue
			}
			b, err := json.Marshal(msgs)
			if err != nil {
				return err
			}
			sections = append(sections, section{Channel: id, Messages: b})
		}
		return emitSections(ctx, sections, "Nothing unread since "+since.Format("2006-01-02 15:04 -07:00")+".")
	},
}

func myUserID(ctx context.Context) (string, error) {
	raw, err := identity(ctx)
	if err != nil {
		return "", err
	}
	return fieldString(raw, "user_id")
}

// unreadIn returns one channel's unread messages, oldest first. A followed
// thread with unread replies appears as its parent carrying only those
// replies, whether or not the parent itself is unread.
func unreadIn(ctx context.Context, channel, me, since string) ([]map[string]any, error) {
	info, err := cli.Call(ctx, "conversations.info", url.Values{"channel": {channel}})
	if err != nil {
		return nil, err
	}
	var env struct {
		Channel struct {
			LastRead string `json:"last_read"`
		} `json:"channel"`
	}
	if err := json.Unmarshal(info, &env); err != nil {
		return nil, err
	}
	lastRead := env.Channel.LastRead
	raw, _, err := channelWindow(ctx, channel, since, "", unreadLimit)
	if err != nil {
		return nil, err
	}
	msgs, err := decodeArray(raw)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, m := range msgs {
		ts, _ := m["ts"].(string)
		unread := lastRead != "" && ts > lastRead && m["user"] != me
		replies, err := unreadReplies(ctx, channel, me, m)
		if err != nil {
			fmt.Fprintf(stderr, "%s thread %s: replies unavailable: %v\n", channel, ts, err)
		}
		if replies != nil {
			m["replies"] = replies
		} else if !unread {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// unreadReplies fetches replies past a followed thread's own read marker.
func unreadReplies(ctx context.Context, channel, me string, parent map[string]any) ([]map[string]any, error) {
	ts, _ := parent["ts"].(string)
	threadRead, _ := parent["last_read"].(string)
	latest, _ := parent["latest_reply"].(string)
	if subscribed, _ := parent["subscribed"].(bool); !subscribed || latest == "" || latest <= threadRead {
		return nil, nil
	}
	raw, _, err := fetchReplies(ctx, channel, ts, 200, url.Values{"oldest": {threadRead}})
	if err != nil {
		return nil, err
	}
	items, err := decodeArray(raw)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, r := range items {
		rts, _ := r["ts"].(string)
		if rts != ts && rts > threadRead && r["user"] != me {
			out = append(out, r)
		}
	}
	return out, nil
}

// ---- mentions ----------------------------------------------------------------

var (
	mentionsSince string
	mentionsLimit int
)

var mentionsCmd = &cobra.Command{
	Use:   "mentions",
	Short: "Messages that mention you (search), newest first",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		since, err := parsePast(mentionsSince, time.Now())
		if err != nil {
			return err
		}
		me, err := myUserID(ctx)
		if err != nil {
			return err
		}
		query := fmt.Sprintf("<@%s> after:%s", me, since.AddDate(0, 0, -1).Format("2006-01-02"))
		result, err := fetchSearch(ctx, query, mentionsLimit, "desc")
		if err != nil {
			return err
		}
		sinceTS := tsOf(since)
		kept := result.Matches[:0]
		for _, raw := range result.Matches {
			var m struct {
				TS string `json:"ts"`
			}
			if json.Unmarshal(raw, &m) == nil && m.TS >= sinceTS {
				kept = append(kept, raw)
			}
		}
		result.Matches = kept
		result.SearchMeta.Returned = len(kept)
		return emitSearchMatches(ctx, result)
	},
}

func init() {
	getCmd.Flags().IntVar(&getContext, "context", 0, "messages to show before and after")
	getCmd.Flags().IntVar(&getRepliesLimit, "replies-limit", 50, "max thread replies shown for a top-level message")

	for _, c := range []*cobra.Command{digestCmd, tailCmd} {
		c.Flags().StringVar(&digestSince, "since", "1d", "start time (2h, 3d, 1w, today, YYYY-MM-DD, …)")
		c.Flags().IntVar(&digestLimit, "limit", 100, "max messages per channel")
		c.Flags().BoolVar(&digestReplies, "replies", true, "expand thread replies inline")
		c.Flags().IntVar(&digestRepliesLimit, "replies-limit", 20, "max replies per thread")
	}
	tailCmd.Flags().BoolVar(&tailPeek, "peek", false, "do not move cursors")
	digestCmd.Flags().StringVar(&digestUntil, "until", "", "end of the window (same forms as --since); default now")

	unreadCmd.Flags().StringVar(&unreadSince, "since", "7d", "how far back to look for activity")
	unreadCmd.Flags().IntVar(&unreadLimit, "limit", 200, "max messages scanned per channel")

	mentionsCmd.Flags().StringVar(&mentionsSince, "since", "7d", "how far back to search")
	mentionsCmd.Flags().IntVar(&mentionsLimit, "limit", 50, "max mentions")

	rootCmd.AddCommand(getCmd, digestCmd, tailCmd, unreadCmd, mentionsCmd)
}
