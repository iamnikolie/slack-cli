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

type transcriptMsg struct {
	User        string            `json:"user"`
	Text        string            `json:"text"`
	TS          string            `json:"ts"`
	ThreadTS    string            `json:"thread_ts"`
	ReplyCount  int               `json:"reply_count"`
	Subtype     string            `json:"subtype"`
	Username    string            `json:"username"`
	BotID       string            `json:"bot_id"`
	Permalink   string            `json:"permalink"`
	Files       []slackFile       `json:"files"`
	Edited      *struct{}         `json:"edited"`
	Reactions   []slackReaction   `json:"reactions"`
	Attachments []slackAttachment `json:"attachments"`
	Blocks      []slackBlock      `json:"blocks"`
	// Replies is filled by `history --replies`; nil means not expanded.
	Replies []transcriptMsg `json:"replies"`
	// Focus marks the message `slk get` was asked about.
	Focus bool `json:"slk_focus"`
}

type slackReaction struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// slackAttachment covers link unfurls (from_url set) and legacy bot
// attachments, whose content often lives here while text stays empty.
type slackAttachment struct {
	ServiceName string `json:"service_name"`
	FromURL     string `json:"from_url"`
	Pretext     string `json:"pretext"`
	Title       string `json:"title"`
	TitleLink   string `json:"title_link"`
	Text        string `json:"text"`
	Fallback    string `json:"fallback"`
}

type slackBlock struct {
	Type string `json:"type"`
	Text *struct {
		Text string `json:"text"`
	} `json:"text"`
	Fields []struct {
		Text string `json:"text"`
	} `json:"fields"`
	Elements []json.RawMessage `json:"elements"`
}

// blocksText recovers readable text from Block Kit for messages that carry an
// empty text fallback (apps and workflows). Rich text from the composer is
// already mirrored in text, so it is only consulted as a last resort.
func blocksText(blocks []slackBlock) string {
	var parts []string
	for _, bl := range blocks {
		if bl.Text != nil && bl.Text.Text != "" {
			parts = append(parts, bl.Text.Text)
		}
		for _, f := range bl.Fields {
			if f.Text != "" {
				parts = append(parts, f.Text)
			}
		}
		if bl.Type == "context" || bl.Type == "rich_text" {
			var sb strings.Builder
			collectRichText(bl.Elements, &sb)
			if t := strings.TrimSpace(sb.String()); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func collectRichText(elems []json.RawMessage, sb *strings.Builder) {
	for _, raw := range elems {
		var e struct {
			Type     string            `json:"type"`
			Text     json.RawMessage   `json:"text"`
			URL      string            `json:"url"`
			Name     string            `json:"name"`
			Elements []json.RawMessage `json:"elements"`
		}
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		var text string
		if json.Unmarshal(e.Text, &text) != nil {
			var obj struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(e.Text, &obj)
			text = obj.Text
		}
		switch {
		case text != "":
			sb.WriteString(text)
		case e.URL != "":
			sb.WriteString(e.URL)
		case e.Type == "emoji" && e.Name != "":
			sb.WriteString(":" + e.Name + ":")
		}
		collectRichText(e.Elements, sb)
	}
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
				fmt.Fprintf(&b, "  ↳ %d %s (ts=%s)\n", m.ReplyCount, plural(m.ReplyCount, "reply", "replies"), m.TS)
			}
			continue
		}
		for _, r := range m.Replies {
			writeTranscriptMsg(&b, r, "    ↳ ", byID, d, channel, baseURL, m.TS)
		}
		if more := m.ReplyCount - len(m.Replies); more > 0 {
			fmt.Fprintf(&b, "    ↳ +%d earlier %s: slk thread %s %s\n", more, plural(more, "reply", "replies"), channel, m.TS)
		}
	}
	return b.String(), nil
}

// writeTranscriptMsg prints one message: a header line carrying date, ts and
// author, then files, attachments, reactions and the link. prefix indents
// thread replies expanded under their parent.
func writeTranscriptMsg(b *strings.Builder, m transcriptMsg, prefix string, byID map[string]string, d *cache.Directory, channel, baseURL, thread string) {
	pad := strings.Repeat(" ", len([]rune(prefix)))
	if m.Focus {
		prefix = "» " + prefix
		pad = "  " + pad
	}
	text := m.Text
	if strings.TrimSpace(text) == "" {
		text = blocksText(m.Blocks)
	}
	author := "@" + transcriptAuthor(m, byID)
	if m.Edited != nil {
		author += " (edited)"
	}
	var attachments []string
	for _, a := range m.Attachments {
		if line := attachmentLine(a, d); line != "" {
			attachments = append(attachments, line)
		}
	}
	if strings.TrimSpace(text) == "" && len(attachments) > 0 {
		// a bot that posts only attachments: its first one is the message
		text, attachments = attachments[0], attachments[1:]
	} else {
		text = truncateText(expandMentions(text, d), maxChars)
	}
	if pad != "" {
		// keep a multi-line reply inside its thread's indent
		text = strings.ReplaceAll(text, "\n", "\n"+pad+"  ")
	}
	fmt.Fprintf(b, "%s[%s] ts=%s %s: %s\n", prefix, tsClock(m.TS), m.TS, author, text)
	for _, file := range m.Files {
		size := fmt.Sprintf("%d bytes", file.Size)
		if file.IsExternal {
			size = "external" // Google Drive and other linked files have no size
		}
		fmt.Fprintf(b, "%s  file %s: %s (%s, %s)\n", pad, file.ID, file.Name, file.MIME, size)
	}
	for _, line := range attachments {
		fmt.Fprintf(b, "%s  ▸ %s\n", pad, line)
	}
	if len(m.Reactions) > 0 {
		rs := make([]string, len(m.Reactions))
		for i, r := range m.Reactions {
			rs[i] = fmt.Sprintf(":%s:×%d", r.Name, r.Count)
		}
		fmt.Fprintf(b, "%s  reactions: %s\n", pad, strings.Join(rs, " "))
	}
	if noLinks {
		return
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

// unfurlPreview caps link-preview text: the link itself is already in the
// message, so the preview only needs to say what it points at.
const unfurlPreview = 200

// attachmentLine renders an attachment on one line. Unfurls show source and
// title; bot attachments show their full content (subject to --max-chars).
func attachmentLine(a slackAttachment, d *cache.Directory) string {
	oneLine := func(s string) string { return strings.Join(strings.Fields(expandMentions(s, d)), " ") }
	if a.FromURL != "" {
		head := a.ServiceName
		if a.Title != "" {
			if head != "" {
				head += ": "
			}
			head += a.Title
		}
		if head == "" {
			head = a.FromURL
		}
		return truncateText(oneLine(head), unfurlPreview)
	}
	var parts []string
	for _, s := range []string{a.Pretext, a.Title, a.Text} {
		if s = oneLine(s); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 && a.Fallback != "" {
		parts = append(parts, oneLine(a.Fallback))
	}
	if a.TitleLink != "" && len(parts) > 0 {
		parts[0] += " (" + a.TitleLink + ")"
	}
	return truncateText(strings.Join(parts, " — "), maxChars)
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

// plural picks the word form for n ("1 reply", "2 replies").
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
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
	d, err := directoryFor(ctx, channel)
	if err != nil {
		return err
	}
	if outputFormat != "" || len(fieldsFlag) > 0 {
		return renderTable(readableText(flattenReplies(raw), d), []string{"ts", "user", "text", "thread_ts", "reply_count"})
	}
	baseURL := ""
	if !noLinks {
		baseURL = workspaceURL(ctx)
	}
	out, err := renderTranscript(raw, d, channel, baseURL, thread)
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(out)
	return err
}

// readableText gives table/CSV rows the transcript's text: Slack markup
// expanded and capped by --max-chars. JSON keeps the raw wire text.
func readableText(raw json.RawMessage, d *cache.Directory) json.RawMessage {
	msgs, err := decodeArray(raw)
	if err != nil {
		return raw
	}
	for _, m := range msgs {
		if text, ok := m["text"].(string); ok {
			m["text"] = truncateText(expandMentions(text, d), maxChars)
		}
	}
	b, err := json.Marshal(msgs)
	if err != nil {
		return raw
	}
	return b
}

// identityCache memoizes auth.test for one process: transcripts need the
// workspace URL for links and unread/mentions need the caller's user ID.
var identityCache json.RawMessage

func identity(ctx context.Context) (json.RawMessage, error) {
	if identityCache != nil {
		return identityCache, nil
	}
	raw, err := cli.Call(ctx, "auth.test", nil)
	if err != nil {
		return nil, err
	}
	identityCache = raw
	return raw, nil
}

// workspaceURL returns the workspace base URL, or "" with a warning; auth.test
// needs no extra scopes, and one lookup avoids a permalink call per message.
func workspaceURL(ctx context.Context) string {
	raw, err := identity(ctx)
	baseURL := ""
	if err == nil {
		baseURL, err = fieldString(raw, "url")
	}
	if err != nil {
		fmt.Fprintf(stderr, "message links unavailable: %v\n", err)
	}
	return baseURL
}

// channelRef resolves ref to a channel ID, refetching the directory once on a
// cache miss (#channel/@... not yet synced).
//
// @user (or a raw user ID) resolves to the DM with that user.
func channelRef(ctx context.Context, ref string) (string, error) {
	if isChannelID(ref) {
		return ref, nil
	}
	if strings.HasPrefix(ref, "@") || isUserID(ref) {
		return dmChannel(ctx, ref)
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
