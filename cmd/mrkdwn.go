package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/iamnikolie/slack-cli/internal/cache"
)

// reAngle matches one Slack control sequence: <@U…>, <#C…|name>, <!here>,
// <!subteam^S…|@team>, <https://…|label>, <mailto:…>.
var reAngle = regexp.MustCompile(`<([^<>\n]+)>`)

var entityReplacer = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&")

// expandMentions rewrites Slack's wire markup into what a reader sees in the
// client: users and channels by name, group and broadcast mentions, links as
// "label (url)", and &lt;/&gt;/&amp; unescaped. Unknown IDs fall back to the
// bare ID so an agent never sees raw <@...> markup.
func expandMentions(text string, d *cache.Directory) string {
	users, channels := map[string]string{}, map[string]string{}
	if d != nil {
		for _, u := range d.Users {
			users[u.ID] = u.Name
		}
		for _, c := range d.Channels {
			channels[c.ID] = c.Name
		}
	}
	out := reAngle.ReplaceAllStringFunc(text, func(m string) string {
		body := m[1 : len(m)-1]
		target, label, hasLabel := strings.Cut(body, "|")
		switch {
		case strings.HasPrefix(target, "@"):
			id := target[1:]
			if name, ok := users[id]; ok {
				return "@" + name
			}
			if hasLabel && label != "" {
				return "@" + strings.TrimPrefix(label, "@")
			}
			return "@" + id
		case strings.HasPrefix(target, "#"):
			id := target[1:]
			if hasLabel && label != "" {
				return "#" + label
			}
			if name, ok := channels[id]; ok && name != "" {
				return "#" + name
			}
			return "#" + id
		case strings.HasPrefix(target, "!"):
			cmd := target[1:]
			if hasLabel && label != "" {
				if strings.HasPrefix(cmd, "subteam^") {
					return "@" + strings.TrimPrefix(label, "@")
				}
				return label // <!date^…|fallback>
			}
			if id, ok := strings.CutPrefix(cmd, "subteam^"); ok {
				return "@" + id
			}
			return "@" + cmd // here, channel, everyone
		default:
			url := entityReplacer.Replace(target)
			if !hasLabel || label == "" {
				return strings.TrimPrefix(url, "mailto:")
			}
			label = entityReplacer.Replace(label)
			if label == url || "mailto:"+label == url {
				return label
			}
			if shortenedURL(label, url) {
				return url
			}
			return fmt.Sprintf("%s (%s)", label, url)
		}
	})
	return entityReplacer.Replace(out)
}

// reAtName matches a typed "@handle" that is not already Slack markup: not
// after "<" or "|" (<@U…>, <url|@label>), and not inside a word or an email
// address. Group 1 is the character before the "@", group 2 the handle.
var reAtName = regexp.MustCompile(`(^|[^\w<|@./-])@([\w][\w.-]*)`)

// reCode matches fenced code blocks and inline code spans, which keep "@" as
// typed.
var reCode = regexp.MustCompile("(?s)```.*?```|`[^`\n]*`")

// linkMentions rewrites "@handle" for each known workspace user into Slack's
// <@U…> mention. Slack renders a typed "@handle" as plain text and notifies
// nobody, in Markdown and mrkdwn alike. Handles match the user name
// case-insensitively; trailing dots and dashes are punctuation. Code keeps its
// text, and unknown handles are left as typed and returned so the caller can
// warn.
func linkMentions(text string, d *cache.Directory) (string, []string) {
	if !strings.Contains(text, "@") || d == nil {
		return text, nil
	}
	ids := make(map[string]string, len(d.Users))
	for _, u := range d.Users {
		if u.Name != "" {
			ids[strings.ToLower(u.Name)] = u.ID
		}
	}
	var unknown []string
	seen := map[string]bool{}
	link := func(s string) string {
		return reAtName.ReplaceAllStringFunc(s, func(m string) string {
			g := reAtName.FindStringSubmatch(m)
			name := g[2]
			tail := ""
			for name != "" {
				if id, ok := ids[strings.ToLower(name)]; ok {
					return g[1] + "<@" + id + ">" + tail
				}
				last := name[len(name)-1]
				if last != '.' && last != '-' {
					break
				}
				name, tail = name[:len(name)-1], string(last)+tail
			}
			switch h := strings.ToLower(g[2]); h {
			case "here", "channel", "everyone":
			default:
				if !seen[h] {
					seen[h] = true
					unknown = append(unknown, "@"+g[2])
				}
			}
			return m
		})
	}
	var b strings.Builder
	last := 0
	for _, loc := range reCode.FindAllStringIndex(text, -1) {
		b.WriteString(link(text[last:loc[0]]))
		b.WriteString(text[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(link(text[last:]))
	return b.String(), unknown
}

// shortenedURL reports whether label is Slack's own display form of url: the
// URL without its scheme, possibly with a middle part elided ("host/a/…/847").
func shortenedURL(label, url string) bool {
	bare := url
	for _, scheme := range []string{"https://", "http://"} {
		bare = strings.TrimPrefix(bare, scheme)
	}
	if label == bare || label == strings.TrimSuffix(bare, "/") {
		return true
	}
	head, tail, elided := strings.Cut(label, "…")
	return elided && head != "" && strings.HasPrefix(bare, head) &&
		strings.HasSuffix(strings.TrimSuffix(bare, "/"), strings.TrimSuffix(tail, "/")) &&
		len(head)+len(tail) < len(bare)
}

// truncateText caps s at max runes (0 = no cap) and says how much was cut.
func truncateText(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return fmt.Sprintf("%s…(+%d chars)", string(r[:max]), len(r)-max)
}
