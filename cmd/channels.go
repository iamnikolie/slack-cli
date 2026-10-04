package cmd

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/spf13/cobra"
)

var (
	channelsTypes  string
	channelsFilter string
	channelsLimit  int
)

var channelsCmd = &cobra.Command{
	Use:     "channels",
	Aliases: []string{"channel"},
	Short:   "List channels from the directory cache",
	RunE: func(cmd *cobra.Command, args []string) error {
		// `channel list` reuses this RunE; `channel view` is a subcommand.
		d, err := loadDirectory(cmd.Context())
		if err != nil {
			return err
		}
		filtered := filterChannels(d.Channels, channelsTypes, channelsFilter, channelsLimit)
		rows := make([]map[string]any, 0, len(filtered))
		for _, c := range filtered {
			b, _ := json.Marshal(c)
			row, err := decodeObject(b)
			if err != nil {
				return err
			}
			row["type"] = channelKind(c)
			rows = append(rows, row)
		}
		b, _ := json.Marshal(rows)
		return emitList(b, []string{"id", "name", "type", "is_member"})
	},
}

var channelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List channels (alias of `slk channels`)",
	RunE:  channelsCmd.RunE,
}

var channelViewCmd = &cobra.Command{
	Use:   "view <#channel|id>",
	Short: "Show channel metadata (conversations.info)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := channelRef(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("channel", id)
		q.Set("include_num_members", "true")
		raw, err := cli.Call(cmd.Context(), "conversations.info", q)
		if err != nil {
			return err
		}
		// conversations.info nests the channel under "channel".
		var env struct {
			Channel json.RawMessage `json:"channel"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return err
		}
		return emitObj(flattenTopics(env.Channel), []string{"id", "name", "is_private", "num_members", "topic", "purpose"})
	},
}

// flattenTopics replaces Slack's topic/purpose objects ({value, creator,
// last_set}) with their text, which is all a reader wants.
func flattenTopics(raw json.RawMessage) json.RawMessage {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return raw
	}
	for _, k := range []string{"topic", "purpose"} {
		if obj, ok := m[k].(map[string]any); ok {
			m[k] = obj["value"]
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return b
}

// channelKind classifies a channel as one of im, mpim, private, or public.
func channelKind(c cache.Channel) string {
	switch {
	case c.IsIM:
		return "im"
	case c.IsMPIM:
		return "mpim"
	case c.IsPrivate:
		return "private"
	default:
		return "public"
	}
}

// filterChannels applies type, substring, and limit filters. types is a
// comma-list of public/private/im/mpim; empty means all.
func filterChannels(in []cache.Channel, types, sub string, limit int) []cache.Channel {
	var want map[string]bool
	if types != "" {
		want = map[string]bool{}
		for _, t := range strings.Split(types, ",") {
			if t = strings.TrimSpace(t); t != "" {
				want[t] = true
			}
		}
	}
	out := []cache.Channel{}
	for _, c := range in {
		if want != nil && !want[channelKind(c)] {
			continue
		}
		if sub != "" && !strings.Contains(c.Name, sub) {
			continue
		}
		out = append(out, c)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func init() {
	channelsCmd.Flags().StringVar(&channelsTypes, "types", "", "comma-list: public,private,im,mpim (default all)")
	channelsCmd.Flags().StringVar(&channelsFilter, "filter", "", "substring match on channel name")
	channelsCmd.Flags().IntVar(&channelsLimit, "limit", 0, "max channels (0 = no cap)")
	channelListCmd.Flags().AddFlagSet(channelsCmd.Flags())
	channelsCmd.AddCommand(channelListCmd, channelViewCmd)
	rootCmd.AddCommand(channelsCmd)
}
