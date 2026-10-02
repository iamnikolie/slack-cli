package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/iamnikolie/slack-cli/internal/client"
	"github.com/spf13/cobra"
)

type channelInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Topic       string            `json:"topic"`
	Purpose     string            `json:"purpose"`
	Members     int               `json:"num_members"`
	Created     string            `json:"created"`
	Creator     string            `json:"creator"`
	Archived    bool              `json:"is_archived"`
	Bookmarks   []channelBookmark `json:"bookmarks"`
	Pins        json.RawMessage   `json:"pins"`
	Unavailable []string          `json:"unavailable,omitempty"`
}

type channelBookmark struct {
	Title string `json:"title"`
	Link  string `json:"link"`
}

var channelInfoCmd = &cobra.Command{
	Use:   "info <#channel|id>",
	Short: "What a channel is for: topic, purpose, members, bookmarks, pinned messages",
	Long: `Topic, purpose, member count, bookmarks (bookmarks:read) and pinned messages
(pins:read). A section the token lacks a scope for is reported, not fatal.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		id, err := channelRef(ctx, args[0])
		if err != nil {
			return err
		}
		info, err := fetchChannelInfo(ctx, id)
		if err != nil {
			return err
		}
		if jsonOutput || outputFormat == "json" {
			b, err := json.Marshal(info)
			if err != nil {
				return err
			}
			return writeRaw(b)
		}
		return printChannelInfo(ctx, info)
	},
}

func fetchChannelInfo(ctx context.Context, id string) (*channelInfo, error) {
	raw, err := cli.Call(ctx, "conversations.info", url.Values{"channel": {id}, "include_num_members": {"true"}})
	if err != nil {
		return nil, err
	}
	var env struct {
		Channel struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Created    int64  `json:"created"`
			Creator    string `json:"creator"`
			IsArchived bool   `json:"is_archived"`
			NumMembers int    `json:"num_members"`
			Topic      struct {
				Value string `json:"value"`
			} `json:"topic"`
			Purpose struct {
				Value string `json:"value"`
			} `json:"purpose"`
		} `json:"channel"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	c := env.Channel
	info := &channelInfo{
		ID: c.ID, Name: c.Name, Topic: c.Topic.Value, Purpose: c.Purpose.Value,
		Members: c.NumMembers, Creator: c.Creator, Archived: c.IsArchived,
		Bookmarks: []channelBookmark{}, Pins: json.RawMessage(`[]`),
	}
	if c.Created > 0 {
		info.Created = time.Unix(c.Created, 0).Format("2006-01-02")
	}

	if raw, err := cli.Call(ctx, "bookmarks.list", url.Values{"channel_id": {id}}); err != nil {
		info.Unavailable = append(info.Unavailable, sectionError("bookmarks", err))
	} else {
		var b struct {
			Bookmarks []channelBookmark `json:"bookmarks"`
		}
		if err := json.Unmarshal(raw, &b); err == nil && b.Bookmarks != nil {
			info.Bookmarks = b.Bookmarks
		}
	}

	if raw, err := cli.Call(ctx, "pins.list", url.Values{"channel": {id}}); err != nil {
		info.Unavailable = append(info.Unavailable, sectionError("pins", err))
	} else {
		var p struct {
			Items []struct {
				Message json.RawMessage `json:"message"`
			} `json:"items"`
		}
		if err := json.Unmarshal(raw, &p); err == nil {
			msgs := make([]json.RawMessage, 0, len(p.Items))
			for _, it := range p.Items {
				if len(it.Message) > 0 {
					msgs = append(msgs, it.Message)
				}
			}
			info.Pins, _ = json.Marshal(msgs)
		}
	}
	return info, nil
}

// sectionError says why an optional section is missing, naming the scope to
// add rather than repeating the full reinstall help.
func sectionError(section string, err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.Code == "missing_scope" {
		return fmt.Sprintf("%s: needs the %s scope", section, apiErr.Needed)
	}
	return fmt.Sprintf("%s: %v", section, err)
}

func printChannelInfo(ctx context.Context, info *channelInfo) error {
	d, err := loadDirectory(ctx)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s (%s)\n\n", profile, channelLabel(d, info.ID), info.ID)
	if info.Topic != "" {
		fmt.Fprintf(&b, "topic: %s\n", expandMentions(info.Topic, d))
	}
	if info.Purpose != "" {
		fmt.Fprintf(&b, "purpose: %s\n", expandMentions(info.Purpose, d))
	}
	line := fmt.Sprintf("members: %d", info.Members)
	if info.Created != "" {
		line += " · created " + info.Created
		if info.Creator != "" {
			line += " by " + expandMentions("<@"+info.Creator+">", d)
		}
	}
	if info.Archived {
		line += " · archived"
	}
	b.WriteString(line + "\n")
	if len(info.Bookmarks) > 0 {
		b.WriteString("\n## Bookmarks\n")
		for _, bm := range info.Bookmarks {
			fmt.Fprintf(&b, "- %s — %s\n", bm.Title, bm.Link)
		}
	}
	var pins []json.RawMessage
	_ = json.Unmarshal(info.Pins, &pins)
	if len(pins) > 0 {
		fmt.Fprintf(&b, "\n## Pinned (%d)\n", len(pins))
		baseURL := ""
		if !noLinks {
			baseURL = workspaceURL(ctx)
		}
		out, err := renderTranscript(info.Pins, d, "", baseURL, "")
		if err != nil {
			return err
		}
		b.WriteString(out)
	}
	for _, u := range info.Unavailable {
		fmt.Fprintf(&b, "\n(%s)", u)
	}
	if len(info.Unavailable) > 0 {
		b.WriteString("\n")
	}
	_, err = os.Stdout.WriteString(b.String())
	return err
}

func init() {
	channelsCmd.AddCommand(channelInfoCmd)
}
