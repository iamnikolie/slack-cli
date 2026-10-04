package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// markdownLimit is Slack's cap on markdown_text.
const markdownLimit = 12000

// outgoing is one message to post, edit or schedule.
type outgoing struct {
	Channel string
	Thread  string
	TS      string    // set for an edit (chat.update)
	At      time.Time // non-zero schedules the message
	Text    string
}

// msgFlags are shared by send, reply, dm and update.
type msgFlags struct {
	bodyFile string
	idOnly   bool
	at       string
	mrkdwn   bool
}

func (f *msgFlags) register(c *cobra.Command, schedule bool) {
	c.Flags().StringVar(&f.bodyFile, "body-file", "", "read text from a file ('-' for stdin)")
	c.Flags().BoolVar(&f.idOnly, "id-only", false, "print only the message ts (scheduled: its id)")
	c.Flags().BoolVar(&f.mrkdwn, "mrkdwn", false, "send text as Slack mrkdwn (*bold*, <url|label>) instead of Markdown")
	if schedule {
		c.Flags().StringVar(&f.at, "at", "", "schedule instead of posting now: +2h, in 30m, 16:00, tomorrow 09:00, YYYY-MM-DD HH:MM")
	}
}

// deliver posts, edits or schedules o. Text goes out as standard Markdown
// (markdown_text) unless --mrkdwn asks for Slack's own syntax. With --dry-run
// nothing is sent and the resolved plan is returned instead.
func deliver(ctx context.Context, o outgoing, f *msgFlags) (json.RawMessage, error) {
	method := "chat.postMessage"
	switch {
	case o.TS != "":
		method = "chat.update"
	case !o.At.IsZero():
		method = "chat.scheduleMessage"
	}
	field := "markdown_text"
	if f.mrkdwn {
		field = "text"
	} else if n := len([]rune(o.Text)); n > markdownLimit {
		return nil, fmt.Errorf("message is %d characters; Markdown messages are capped at %d (split it, or pass --mrkdwn)", n, markdownLimit)
	}
	q := url.Values{"channel": {o.Channel}, field: {o.Text}}
	if o.TS != "" {
		q.Set("ts", o.TS)
	}
	if o.Thread != "" {
		q.Set("thread_ts", o.Thread)
	}
	if !o.At.IsZero() {
		if !o.At.After(time.Now()) {
			return nil, fmt.Errorf("--at %s is in the past", o.At.Format("2006-01-02 15:04 -07:00"))
		}
		q.Set("post_at", strconv.FormatInt(o.At.Unix(), 10))
	}
	if dryRun {
		plan := map[string]any{"dry_run": true, "method": method, "channel": o.Channel, "format": field, "text": o.Text}
		if d, err := directoryFor(ctx, o.Channel); err == nil {
			plan["channel_name"] = channelLabel(d, o.Channel)
		}
		for k, v := range map[string]string{"ts": o.TS, "thread_ts": o.Thread} {
			if v != "" {
				plan[k] = v
			}
		}
		if !o.At.IsZero() {
			plan["post_at"] = o.At.Format("2006-01-02 15:04 -07:00")
		}
		return json.Marshal(plan)
	}
	return cli.Call(ctx, method, q)
}

// emitDelivered prints a delivery result (or dry-run plan).
func emitDelivered(raw json.RawMessage, f *msgFlags) error {
	if dryRun {
		return emitObj(raw, []string{"dry_run", "method", "channel", "channel_name", "thread_ts", "ts", "post_at", "format", "text"})
	}
	if _, err := fieldString(raw, "scheduled_message_id"); err == nil {
		if f.idOnly {
			return printIDOnly(raw, "scheduled_message_id")
		}
		m, err := decodeObject(raw)
		if err != nil {
			return err
		}
		if at, ok := m["post_at"].(json.Number); ok {
			if n, err := at.Int64(); err == nil {
				m["post_at"] = time.Unix(n, 0).Format("2006-01-02 15:04 -07:00")
			}
		}
		b, _ := json.Marshal(m)
		return emitObj(b, []string{"ok", "channel", "scheduled_message_id", "post_at"})
	}
	if f.idOnly {
		return printIDOnly(raw, "ts")
	}
	return emitObj(raw, []string{"ok", "channel", "ts"})
}

// runMessage is the shared body of send/reply/dm: read text, resolve --at,
// deliver, print.
func runMessage(ctx context.Context, channel, thread, text string, f *msgFlags) error {
	body, err := readBody(text, f.bodyFile)
	if err != nil {
		return err
	}
	o := outgoing{Channel: channel, Thread: thread, Text: body}
	if f.at != "" {
		if o.At, err = parseFuture(f.at, time.Now()); err != nil {
			return err
		}
	}
	raw, err := deliver(ctx, o, f)
	if err != nil {
		return err
	}
	return emitDelivered(raw, f)
}

// dmChannel returns the DM channel with a user: from the directory when one
// exists, otherwise opened via conversations.open (needs im:write).
func dmChannel(ctx context.Context, ref string) (string, error) {
	uid, err := userRef(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("unknown user %q: %w", ref, err)
	}
	d, err := loadDirectory(ctx)
	if err != nil {
		return "", err
	}
	for _, c := range d.Channels {
		if c.IsIM && c.User == uid {
			return c.ID, nil
		}
	}
	raw, err := cli.Call(ctx, "conversations.open", url.Values{"users": {uid}, "return_im": {"true"}})
	if err != nil {
		return "", err
	}
	var env struct {
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Channel.ID == "" {
		return "", fmt.Errorf("conversations.open returned no channel")
	}
	return env.Channel.ID, nil
}

// ---- dm / react / scheduled ----------------------------------------------------

var dmFlags msgFlags

var dmCmd = &cobra.Command{
	Use:   "dm <@user> [text]",
	Short: "Direct-message a user (opens the DM if needed; that needs im:write)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]
		if !strings.HasPrefix(ref, "@") && !isUserID(ref) {
			ref = "@" + ref
		}
		id, err := dmChannel(cmd.Context(), ref)
		if err != nil {
			return err
		}
		text := ""
		if len(args) == 2 {
			text = args[1]
		}
		return runMessage(cmd.Context(), id, "", text, &dmFlags)
	},
}

var reactRemove bool

var reactCmd = &cobra.Command{
	Use:   "react <permalink | #channel ts> <emoji>",
	Short: "Add an emoji reaction (needs reactions:write); --remove takes it back",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReact(cmd.Context(), args, reactRemove)
	},
}

var unreactCmd = &cobra.Command{
	Use:   "unreact <permalink | #channel ts> <emoji>",
	Short: "Remove your emoji reaction (same as react --remove)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReact(cmd.Context(), args, true)
	},
}

func runReact(ctx context.Context, args []string, remove bool) error {
	emoji := strings.Trim(args[len(args)-1], ":")
	if emoji == "" {
		return fmt.Errorf("emoji name required (e.g. :+1: or eyes)")
	}
	target := args[:len(args)-1]
	ref := target[len(target)-1]
	ch, ts, ok := parseMessageRef(ref) // a reaction targets the exact message, not its thread
	if !ok {
		return errInvalidTS(ref)
	}
	if len(target) == 2 {
		var err error
		if ch, err = channelRef(ctx, target[0]); err != nil {
			return err
		}
	}
	if ch == "" {
		return fmt.Errorf("with a bare ts, give the channel: slk react <#channel> <ts> <emoji>")
	}
	method := "reactions.add"
	if remove {
		method = "reactions.remove"
	}
	if dryRun {
		plan := map[string]any{"dry_run": true, "method": method, "channel": ch, "ts": ts, "emoji": emoji}
		if d, err := directoryFor(ctx, ch); err == nil {
			plan["channel_name"] = channelLabel(d, ch)
		}
		b, _ := json.Marshal(plan)
		return emitObj(b, []string{"dry_run", "method", "channel", "channel_name", "ts", "emoji"})
	}
	if _, err := cli.Call(ctx, method, url.Values{"channel": {ch}, "timestamp": {ts}, "name": {emoji}}); err != nil {
		return err
	}
	b, _ := json.Marshal(map[string]any{"ok": true, "channel": ch, "ts": ts, "emoji": ":" + emoji + ":", "removed": remove})
	return emitObj(b, []string{"ok", "channel", "ts", "emoji", "removed"})
}

var scheduledCmd = &cobra.Command{
	Use:   "scheduled [#channel]",
	Short: "List your scheduled messages (send/reply/dm --at creates them)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		q := url.Values{}
		if len(args) == 1 {
			id, err := channelRef(ctx, args[0])
			if err != nil {
				return err
			}
			q.Set("channel", id)
		}
		raw, _, err := cli.Paginate(ctx, "chat.scheduledMessages.list", q, "scheduled_messages", 1000)
		if err != nil {
			return err
		}
		if jsonOutput || outputFormat == "json" {
			return writeRaw(projectList(raw, fieldsFlag))
		}
		d, err := loadDirectory(ctx)
		if err != nil {
			return err
		}
		items, err := decodeArray(raw)
		if err != nil {
			return err
		}
		for _, it := range items {
			if n, ok := it["post_at"].(json.Number); ok {
				if sec, err := n.Int64(); err == nil {
					it["post_at"] = time.Unix(sec, 0).Format("2006-01-02 15:04 -07:00")
				}
			}
			if id, _ := it["channel_id"].(string); id != "" {
				it["channel"] = channelLabel(d, id)
			}
			text, _ := it["text"].(string)
			if text == "" {
				text = "(Markdown message; Slack does not return its text)"
			}
			it["text"] = truncateText(expandMentions(text, d), maxChars)
		}
		b, _ := json.Marshal(items)
		return emitList(b, []string{"id", "channel", "channel_id", "post_at", "text"})
	},
}

var scheduledDeleteCmd = &cobra.Command{
	Use:   "delete <#channel> <scheduled-id>",
	Short: "Cancel a scheduled message; requires --yes",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		id, err := channelRef(ctx, args[0])
		if err != nil {
			return err
		}
		if dryRun {
			b, _ := json.Marshal(map[string]any{"dry_run": true, "method": "chat.deleteScheduledMessage", "channel": id, "scheduled_message_id": args[1]})
			return emitObj(b, nil)
		}
		if !assumeYes {
			return fmt.Errorf("refusing to cancel without --yes")
		}
		if _, err := cli.Call(ctx, "chat.deleteScheduledMessage", url.Values{"channel": {id}, "scheduled_message_id": {args[1]}}); err != nil {
			return err
		}
		b, _ := json.Marshal(map[string]any{"ok": true, "channel": id, "scheduled_message_id": args[1]})
		return emitObj(b, []string{"ok", "channel", "scheduled_message_id"})
	},
}

func init() {
	dmFlags.register(dmCmd, true)
	reactCmd.Flags().BoolVar(&reactRemove, "remove", false, "remove the reaction instead")
	scheduledCmd.AddCommand(scheduledDeleteCmd)
	rootCmd.AddCommand(dmCmd, reactCmd, unreactCmd, scheduledCmd)
}
