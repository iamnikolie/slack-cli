package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/iamnikolie/slack-cli/internal/cache"
)

// errNotInCache signals a #channel/@user not found in the directory cache;
// callers may refetch once and retry.
var errNotInCache = errors.New("not found in directory cache")

var (
	reChannelID = regexp.MustCompile(`^[CDG][A-Z0-9]{6,}$`)
	reUserID    = regexp.MustCompile(`^[UW][A-Z0-9]{6,}$`)
	reTS        = regexp.MustCompile(`^\d{10}\.\d{6}$`)
	rePermalink = regexp.MustCompile(`/archives/([CDG][A-Z0-9]+)/p(\d{16})`)
	// A reply's permalink carries its parent as ?thread_ts=1700000000.000100.
	rePermalinkThread = regexp.MustCompile(`[?&]thread_ts=(\d{10}\.\d{6})`)
)

func isChannelID(s string) bool { return reChannelID.MatchString(s) }
func isUserID(s string) bool    { return reUserID.MatchString(s) }

// resolveChannel maps a raw ID / #name / name to a channel ID.
func resolveChannel(d *cache.Directory, ref string) (string, error) {
	if isChannelID(ref) {
		return ref, nil
	}
	name := strings.TrimPrefix(ref, "#")
	if d != nil {
		for _, c := range d.Channels {
			if c.Name == name {
				return c.ID, nil
			}
		}
	}
	return "", errNotInCache
}

// resolveUser maps a raw ID / @name / name / real name to a user ID.
func resolveUser(d *cache.Directory, ref string) (string, error) {
	if isUserID(ref) {
		return ref, nil
	}
	name := strings.TrimPrefix(ref, "@")
	if d != nil {
		for _, u := range d.Users {
			if u.Name == name || u.RealName == name {
				return u.ID, nil
			}
		}
	}
	return "", errNotInCache
}

// errInvalidTS returns a consistent error for a malformed message ts.
func errInvalidTS(s string) error {
	return fmt.Errorf("invalid ts %q (expected 1700000000.000100 or a permalink)", s)
}

// parseMessageRef accepts a Slack permalink or a bare ts. For a permalink it
// returns (channel, ts, true); for a bare ts it returns ("", ts, true).
func parseMessageRef(ref string) (channel, ts string, ok bool) {
	if m := rePermalink.FindStringSubmatch(ref); m != nil {
		return m[1], permalinkDigitsToTS(m[2]), true
	}
	if reTS.MatchString(ref) {
		return "", ref, true
	}
	return "", "", false
}

// parseThreadRef is parseMessageRef for thread targets. Threads hang off the
// parent message, so a reply's permalink resolves to its ?thread_ts= parent
// rather than the reply's own ts.
func parseThreadRef(ref string) (channel, ts string, ok bool) {
	channel, ts, ok = parseMessageRef(ref)
	if ok && channel != "" {
		if m := rePermalinkThread.FindStringSubmatch(ref); m != nil {
			ts = m[1]
		}
	}
	return channel, ts, ok
}

// permalinkDigitsToTS turns the 16-digit "p1700000000123456" tail into the
// "1700000000.123456" ts form (dot 6 digits from the end).
func permalinkDigitsToTS(digits string) string {
	if len(digits) <= 6 {
		return digits
	}
	return digits[:len(digits)-6] + "." + digits[len(digits)-6:]
}

// reMention matches Slack user mentions like <@U12345>.
var reMention = regexp.MustCompile(`<@([UW][A-Z0-9]+)>`)

// expandMentions rewrites <@Uxxx> to @name using the user cache; unknown IDs
// fall back to the bare ID so an agent never sees raw <@...> markup.
func expandMentions(text string, d *cache.Directory) string {
	if d == nil {
		return text
	}
	byID := map[string]string{}
	for _, u := range d.Users {
		byID[u.ID] = u.Name
	}
	return reMention.ReplaceAllStringFunc(text, func(m string) string {
		id := reMention.FindStringSubmatch(m)[1]
		if name, ok := byID[id]; ok {
			return "@" + name
		}
		return "@" + id
	})
}

type transcriptMsg struct {
	User       string      `json:"user"`
	Text       string      `json:"text"`
	TS         string      `json:"ts"`
	ThreadTS   string      `json:"thread_ts"`
	ReplyCount int         `json:"reply_count"`
	Subtype    string      `json:"subtype"`
	Username   string      `json:"username"`
	BotID      string      `json:"bot_id"`
	Permalink  string      `json:"permalink"`
	Files      []slackFile `json:"files"`
	// Replies is filled by `history --replies`; nil means not expanded.
	Replies []transcriptMsg `json:"replies"`
}

// transcriptAuthor picks a display label for a message author, falling back
// through user → username (bots/apps) → bot id → subtype → "system" so system
// and bot messages never render as a bare "@".
func transcriptAuthor(m transcriptMsg, byID map[string]string) string {
	if m.User != "" {
		if n, ok := byID[m.User]; ok {
			return n
		}
		return m.User
	}
	if m.Username != "" {
		return m.Username
	}
	if m.BotID != "" {
		return "bot:" + m.BotID
	}
	if m.Subtype != "" {
		return m.Subtype
	}
	return "system"
}

// renderTranscript turns a JSON array of messages into a readable transcript:
//
//	[YYYY-MM-DD HH:MM:SS offset] @user: text                ts=<ts>
//	  ↳ N replies (ts=<ts>)
func renderTranscript(data json.RawMessage, d *cache.Directory, channel, baseURL, thread string) (string, error) {
	var msgs []transcriptMsg
	if err := json.Unmarshal(data, &msgs); err != nil {
		return "", fmt.Errorf("renderTranscript: %w", err)
	}
	byID := map[string]string{}
	if d != nil {
		for _, u := range d.Users {
			byID[u.ID] = u.Name
		}
	}
	var b strings.Builder
	if channel != "" {
		fmt.Fprintf(&b, "# %s — %s (%s)", profile, channelLabel(d, channel), channel)
		if thread != "" {
			fmt.Fprintf(&b, " — thread %s", thread)
		}
		b.WriteString("\n\n")
	}
	if len(msgs) == 0 {
		b.WriteString("No messages.\n")
	}
	for _, m := range msgs {
		writeTranscriptMsg(&b, m, "", byID, d, channel, baseURL, thread)
		if m.Replies == nil {
			if m.ReplyCount > 0 {
				fmt.Fprintf(&b, "  ↳ %d replies (ts=%s)\n", m.ReplyCount, m.TS)
			}
			continue
		}
		for _, r := range m.Replies {
			writeTranscriptMsg(&b, r, "    ↳ ", byID, d, channel, baseURL, m.TS)
		}
		if more := m.ReplyCount - len(m.Replies); more > 0 {
			fmt.Fprintf(&b, "    ↳ +%d more replies: slk thread %s %s\n", more, channel, m.TS)
		}
	}
	return b.String(), nil
}

// writeTranscriptMsg prints one message line plus its files and link; prefix
// indents thread replies expanded under their parent.
func writeTranscriptMsg(b *strings.Builder, m transcriptMsg, prefix string, byID map[string]string, d *cache.Directory, channel, baseURL, thread string) {
	pad := strings.Repeat(" ", len([]rune(prefix)))
	fmt.Fprintf(b, "%s[%s] @%s: %s\tts=%s\n", prefix, tsClock(m.TS), transcriptAuthor(m, byID), expandMentions(m.Text, d), m.TS)
	for _, file := range m.Files {
		fmt.Fprintf(b, "%s  file %s: %s (%s, %d bytes)\n", pad, file.ID, file.Name, file.MIME, file.Size)
	}
	link := m.Permalink
	if link == "" {
		threadTS := m.ThreadTS
		if threadTS == "" {
			threadTS = thread
		}
		link = messagePermalink(baseURL, channel, m.TS, threadTS)
	}
	if link != "" {
		fmt.Fprintf(b, "%s  %s\n", pad, link)
	}
}

// tsClock includes the date and UTC offset so old messages cannot look current.
func tsClock(ts string) string {
	sec, _, _ := strings.Cut(ts, ".")
	n, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		return "unknown date"
	}
	return time.Unix(n, 0).Format("2006-01-02 15:04:05 -07:00")
}

func channelLabel(d *cache.Directory, id string) string {
	if d != nil {
		for _, c := range d.Channels {
			if c.ID == id && c.Name != "" {
				if c.IsIM {
					return "@" + c.Name
				}
				return "#" + c.Name
			}
		}
	}
	return id
}

func messagePermalink(baseURL, channel, ts, thread string) string {
	if baseURL == "" || channel == "" || !reTS.MatchString(ts) {
		return ""
	}
	link := strings.TrimRight(baseURL, "/") + "/archives/" + channel + "/p" + strings.ReplaceAll(ts, ".", "")
	if thread != "" && thread != ts {
		link += "?thread_ts=" + url.QueryEscape(thread) + "&cid=" + url.QueryEscape(channel)
	}
	return link
}

// emitTranscript keeps machine formats free of headings and extra API calls.
func emitTranscript(ctx context.Context, raw json.RawMessage, channel, thread string) error {
	if jsonOutput || outputFormat == "json" {
		return writeRaw(projectList(raw, fieldsFlag))
	}
	if outputFormat != "" || len(fieldsFlag) > 0 {
		return renderTable(flattenReplies(raw), []string{"ts", "user", "text", "thread_ts", "reply_count"})
	}
	d, err := loadDirectory(ctx)
	if err != nil {
		return err
	}
	// auth.test supplies the workspace URL without requiring additional scopes.
	// One lookup per transcript avoids a permalink API call for every message.
	baseURL := ""
	identity, err := cli.Call(ctx, "auth.test", nil)
	if err == nil {
		baseURL, err = fieldString(identity, "url")
	}
	if err != nil {
		fmt.Fprintf(stderr, "message links unavailable: %v\n", err)
	}
	out, err := renderTranscript(raw, d, channel, baseURL, thread)
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(out)
	return err
}

// channelRef resolves ref to a channel ID, refetching the directory once on a
// cache miss (#channel/@... not yet synced).
func channelRef(ctx context.Context, ref string) (string, error) {
	if isChannelID(ref) {
		return ref, nil
	}
	d, err := loadDirectory(ctx)
	if err != nil {
		return "", err
	}
	id, err := resolveChannel(d, ref)
	if err == errNotInCache {
		if d, err = fetchDirectory(ctx); err != nil {
			return "", err
		}
		dir = d
		return resolveChannel(d, ref)
	}
	return id, err
}

// userRef resolves ref to a user ID, refetching once on a cache miss.
func userRef(ctx context.Context, ref string) (string, error) {
	d, err := loadDirectory(ctx)
	if err != nil {
		return "", err
	}
	id, err := resolveUser(d, ref)
	if err == errNotInCache {
		if d, err = fetchDirectory(ctx); err != nil {
			return "", err
		}
		dir = d
		return resolveUser(d, ref)
	}
	return id, err
}
