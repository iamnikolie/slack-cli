package cmd

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var (
	historyLimit  int
	historyOldest string
	historyLatest string
	historySince  string
	historyUntil  string
)

// parseTimeBound accepts a Slack ts (10.6), epoch seconds, or YYYY-MM-DD (UTC)
// and returns the value Slack's oldest/latest expects (epoch seconds string,
// or the ts unchanged).
func parseTimeBound(s string) (string, error) {
	if reTS.MatchString(s) {
		return s, nil
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return s, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return strconv.FormatInt(t.UTC().Unix(), 10), nil
	}
	return "", fmt.Errorf("cannot parse time %q: use a ts, epoch seconds, or YYYY-MM-DD", s)
}

var historyCmd = &cobra.Command{
	Use:   "history <#channel|id>",
	Short: "Read channel message history (transcript)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := channelRef(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		q := url.Values{}
		q.Set("channel", id)
		if historyOldest != "" {
			q.Set("oldest", historyOldest)
		}
		if historyLatest != "" {
			q.Set("latest", historyLatest)
		}
		if historySince != "" {
			v, err := parseTimeBound(historySince)
			if err != nil {
				return err
			}
			q.Set("oldest", v)
		}
		if historyUntil != "" {
			v, err := parseTimeBound(historyUntil)
			if err != nil {
				return err
			}
			q.Set("latest", v)
		}
		raw, hit, err := cli.Paginate(cmd.Context(), "conversations.history", q, "messages", historyLimit)
		if err != nil {
			return err
		}
		paginationHint(stderr, hit, historyLimit)

		return emitTranscript(cmd.Context(), raw, id, "")
	},
}

func init() {
	historyCmd.Flags().IntVar(&historyLimit, "limit", 50, "max messages")
	historyCmd.Flags().StringVar(&historyOldest, "oldest", "", "only messages after this ts (epoch or ts)")
	historyCmd.Flags().StringVar(&historyLatest, "latest", "", "only messages before this ts (epoch or ts)")
	historyCmd.Flags().StringVar(&historySince, "since", "", "only messages on/after this date (YYYY-MM-DD, epoch, or ts)")
	historyCmd.Flags().StringVar(&historyUntil, "until", "", "only messages before this date (YYYY-MM-DD, epoch, or ts)")
	rootCmd.AddCommand(historyCmd)
}
