package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/spf13/cobra"
)

// fetchDirectory pulls all channels (public/private/im/mpim) and users from the
// Slack API and writes the directory cache.
func fetchDirectory(ctx context.Context) (*cache.Directory, error) {
	chParams := url.Values{}
	chParams.Set("types", "public_channel,private_channel,mpim,im")
	chParams.Set("exclude_archived", "true")
	chRaw, _, err := cli.Paginate(ctx, "conversations.list", chParams, "channels", 5000)
	if err != nil {
		return nil, err
	}
	var channels []cache.Channel
	if err := json.Unmarshal(chRaw, &channels); err != nil {
		return nil, fmt.Errorf("sync: decode channels: %w", err)
	}

	usRaw, _, err := cli.Paginate(ctx, "users.list", url.Values{}, "members", 10000)
	if err != nil {
		return nil, err
	}
	var rawUsers []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		RealName string `json:"real_name"`
		Deleted  bool   `json:"deleted"`
		IsBot    bool   `json:"is_bot"`
	}
	if err := json.Unmarshal(usRaw, &rawUsers); err != nil {
		return nil, fmt.Errorf("sync: decode users: %w", err)
	}
	// Drop deleted accounts and bots so name resolution and listings stay clean.
	users := make([]cache.User, 0, len(rawUsers))
	userByID := make(map[string]string, len(rawUsers))
	for _, u := range rawUsers {
		userByID[u.ID] = u.Name // index everyone for DM-name backfill
		if u.Deleted || u.IsBot {
			continue
		}
		users = append(users, cache.User{ID: u.ID, Name: u.Name, RealName: u.RealName})
	}
	// IM (DM) channels have no name; backfill from the partner's handle so they
	// list and resolve like "@partner".
	for i := range channels {
		if channels[i].IsIM && channels[i].Name == "" && channels[i].User != "" {
			if n, ok := userByID[channels[i].User]; ok {
				channels[i].Name = n
			}
		}
	}

	d := &cache.Directory{Channels: channels, Users: users}
	if err := cache.Save(d, profile); err != nil {
		return nil, err
	}
	return d, nil
}

// loadDirectory returns the cached directory, fetching it on a cold cache.
func loadDirectory(ctx context.Context) (*cache.Directory, error) {
	if dir != nil {
		return dir, nil
	}
	d, err := cache.Load(profile)
	if err != nil {
		if cache.IsNotExist(err) {
			d, err = fetchDirectory(ctx)
		}
		if err != nil {
			return nil, err
		}
	}
	dir = d
	return dir, nil
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Refresh the cached channel + user directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := fetchDirectory(cmd.Context())
		if err != nil {
			return err
		}
		dir = d
		fmt.Fprintf(stderr, "synced %d channels, %d users\n", len(d.Channels), len(d.Users))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
