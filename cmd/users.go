package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/spf13/cobra"
)

var usersFilter string

var usersCmd = &cobra.Command{
	Use:     "users [@user]",
	Aliases: []string{"user"},
	Short:   "List users from the directory cache; with @user, show that profile",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return userViewCmd.RunE(cmd, args)
		}
		for _, f := range fieldsFlag {
			if f != "id" && f != "name" && f != "real_name" {
				return fmt.Errorf("users lists the directory cache, which holds id, name, real_name only; for %q use 'slk users @user'", f)
			}
		}
		d, err := loadDirectory(cmd.Context())
		if err != nil {
			return err
		}
		out := []cache.User{}
		for _, u := range d.Users {
			if usersFilter != "" &&
				!strings.Contains(u.Name, usersFilter) &&
				!strings.Contains(u.RealName, usersFilter) {
				continue
			}
			out = append(out, u)
		}
		b, _ := json.Marshal(out)
		return emitList(b, []string{"id", "name", "real_name"})
	},
}

var userViewCmd = &cobra.Command{
	Use:   "view <@user|id>",
	Short: "Show a user profile (users.info)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := userRef(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("user", id)
		raw, err := cli.Call(cmd.Context(), "users.info", q)
		if err != nil {
			return err
		}
		var env struct {
			User map[string]any `json:"user"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return err
		}
		describeUser(cmd.Context(), env.User, time.Now())
		b, err := json.Marshal(env.User)
		if err != nil {
			return err
		}
		return emitObj(b, []string{"id", "name", "real_name", "title", "status", "presence", "local_time", "tz", "is_admin", "is_bot", "deleted"})
	},
}

// describeUser adds what an agent asks before pinging someone: their local
// time, title, status and presence (presence is best-effort).
func describeUser(ctx context.Context, u map[string]any, now time.Time) {
	if off, ok := u["tz_offset"].(float64); ok {
		u["local_time"] = now.In(time.FixedZone("", int(off))).Format("2006-01-02 15:04 Mon")
	}
	if p, ok := u["profile"].(map[string]any); ok {
		u["title"] = p["title"]
		status := strings.TrimSpace(fmt.Sprintf("%s %s", p["status_emoji"], p["status_text"]))
		if exp, ok := p["status_expiration"].(float64); ok && exp > 0 && status != "" {
			status += " (until " + time.Unix(int64(exp), 0).Format("2006-01-02 15:04") + ")"
		}
		u["status"] = status
	}
	if id, _ := u["id"].(string); id != "" && cli != nil {
		if raw, err := cli.Call(ctx, "users.getPresence", url.Values{"user": {id}}); err == nil {
			if p, err := fieldString(raw, "presence"); err == nil {
				u["presence"] = p
			}
		}
	}
}

func init() {
	usersCmd.Flags().StringVar(&usersFilter, "filter", "", "substring match on name/real name")
	usersCmd.AddCommand(userViewCmd)
	rootCmd.AddCommand(usersCmd)
}
