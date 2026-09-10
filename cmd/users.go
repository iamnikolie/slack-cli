package cmd

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/spf13/cobra"
)

var usersFilter string

var usersCmd = &cobra.Command{
	Use:     "users",
	Aliases: []string{"user"},
	Short:   "List users from the directory cache",
	RunE: func(cmd *cobra.Command, args []string) error {
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
			User json.RawMessage `json:"user"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return err
		}
		return emitObj(env.User, []string{"id", "name", "real_name", "tz", "is_admin", "is_bot"})
	},
}

func init() {
	usersCmd.Flags().StringVar(&usersFilter, "filter", "", "substring match on name/real name")
	usersCmd.AddCommand(userViewCmd)
	rootCmd.AddCommand(usersCmd)
}
