package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/spf13/cobra"
)

var activityLimit int
var activitySince string

type activityRow struct {
	Channel         string `json:"channel"`
	ID              string `json:"id"`
	Type            string `json:"type"`
	Latest          string `json:"latest"`
	LatestTS        string `json:"latest_ts"`
	SampledMessages int    `json:"sampled_messages"`
	Preview         string `json:"preview"`
	Permalink       string `json:"permalink"`
}

// summarizeActivity counts only the retrieved search sample, not channel totals.
func summarizeActivity(matches []json.RawMessage, d *cache.Directory) ([]activityRow, error) {
	byChannel := map[string]*activityRow{}
	for _, raw := range matches {
		var m struct {
			transcriptMsg
			Channel struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"channel"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		if m.Channel.ID == "" {
			continue
		}
		row := byChannel[m.Channel.ID]
		if row == nil {
			row = &activityRow{ID: m.Channel.ID, Channel: m.Channel.Name, Type: "unknown"}
			if row.Channel == "" {
				row.Channel = row.ID
			}
			if d != nil {
				for _, c := range d.Channels {
					if c.ID == row.ID {
						row.Type = channelKind(c)
						row.Channel = channelLabel(d, row.ID)
						break
					}
				}
			}
			byChannel[row.ID] = row
		}
		row.SampledMessages++
		if m.TS > row.LatestTS {
			row.LatestTS = m.TS
			row.Latest = tsClock(m.TS)
			row.Permalink = m.Permalink
			preview := []rune(strings.Join(strings.Fields(expandMentions(m.Text, d)), " "))
			if len(preview) > 160 {
				preview = append(preview[:160], '…')
			}
			row.Preview = string(preview)
		}
	}
	rows := make([]activityRow, 0, len(byChannel))
	for _, row := range byChannel {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LatestTS == rows[j].LatestTS {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].LatestTS > rows[j].LatestTS
	})
	return rows, nil
}

var activityCmd = &cobra.Command{
	Use:   "activity",
	Short: "Overview by channel from a sample of the latest indexed messages",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := time.Parse("2006-01-02", activitySince); err != nil {
			return fmt.Errorf("--since must be YYYY-MM-DD")
		}
		query := "after:" + activitySince
		result, err := fetchSearch(cmd.Context(), query, activityLimit, "desc")
		if err != nil {
			return err
		}
		matches, total := result.Matches, result.SearchMeta.Total
		d, err := loadDirectory(cmd.Context())
		if err != nil {
			return err
		}
		rows, err := summarizeActivity(matches, d)
		if err != nil {
			return err
		}
		if jsonOutput || outputFormat == "json" {
			data, err := json.Marshal(struct {
				Query        string        `json:"query"`
				SampleSize   int           `json:"sample_size"`
				TotalMatches int           `json:"total_matches"`
				Truncated    bool          `json:"truncated"`
				Channels     []activityRow `json:"channels"`
			}{query, len(matches), total, result.SearchMeta.HasMore, rows})
			if err != nil {
				return err
			}
			return emitObj(data, nil)
		}
		fmt.Fprintf(stderr, "Activity [%s]: latest %d of %d indexed matches (%s). Counts cover this sample only; search visibility/indexing may omit messages.\n", profile, len(matches), total, query)
		searchPaginationHint(result.SearchMeta, activityLimit)
		data, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		return emitList(data, []string{"channel", "type", "latest", "sampled_messages", "preview", "permalink"})
	},
}

func init() {
	activityCmd.Flags().IntVar(&activityLimit, "limit", 200, "max indexed messages to sample (not channel count)")
	activityCmd.Flags().StringVar(&activitySince, "since", "1970-01-01", "search after YYYY-MM-DD; default all dates")
	rootCmd.AddCommand(activityCmd)
}
