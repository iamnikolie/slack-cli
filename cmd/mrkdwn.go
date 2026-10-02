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
			return fmt.Sprintf("%s (%s)", label, url)
		}
	})
	return entityReplacer.Replace(out)
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
