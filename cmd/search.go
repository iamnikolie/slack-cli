package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	searchIn       string
	searchFrom     string
	searchTo       string
	searchSince    string
	searchUntil    string
	searchOn       string
	searchHas      []string
	searchThreads  bool
	searchLimit    int
	searchOrder    string
	searchWithMeta bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search messages across the workspace (search.messages)",
	Long:  "Search messages across pages up to --limit (1–10000, at most 100 per page).\nAlways reports total, returned, has_more and has_next_page on stderr.\nUse --json --with-meta for {matches,meta}; --fields projects matches only.\nSlack search totals describe indexed matches, not every message in channel history.",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		query, err := buildSearchQuery(strings.Join(args, " "), searchFilters{
			In: searchIn, From: searchFrom, To: searchTo, Since: searchSince, Until: searchUntil,
			On: searchOn, Has: searchHas, Threads: searchThreads,
		}, time.Now())
		if err != nil {
			return err
		}
		if searchWithMeta && !jsonOutput && outputFormat != "json" {
			return fmt.Errorf("--with-meta requires --json or --format json")
		}
		result, err := fetchSearch(cmd.Context(), query, searchLimit, searchOrder)
		if err != nil {
			return err
		}
		meta := result.SearchMeta
		fmt.Fprintf(stderr, "Search: total=%d returned=%d has_more=%t has_next_page=%t pages_fetched=%d page_count=%d omitted_from_last_page=%d\n", meta.Total, meta.Returned, meta.HasMore, meta.HasNextPage, meta.PagesFetched, meta.PageCount, meta.OmittedFromLastPage)
		searchPaginationHint(meta, searchLimit)
		return emitSearchMatches(cmd.Context(), result)
	},
}

// emitSearchMatches prints matches as a table (decoded text, DM partners by
// name) or, with --json, as raw matches or the --with-meta envelope.
func emitSearchMatches(ctx context.Context, result searchResult) error {
	b, err := json.Marshal(result.Matches)
	if err != nil {
		return err
	}
	if jsonOutput || outputFormat == "json" {
		if searchWithMeta {
			return emitSearchEnvelope(result)
		}
		return emitList(b, nil)
	}
	d, err := loadDirectory(ctx)
	if err != nil {
		return err
	}
	items, err := decodeArray(b)
	if err != nil {
		return err
	}
	for _, item := range items {
		if ts, ok := item["ts"].(string); ok {
			item["date"] = tsClock(ts)
		}
		if item["username"] == nil || item["username"] == "" {
			item["username"] = item["user"]
		}
		if text, ok := item["text"].(string); ok {
			item["text"] = truncateText(expandMentions(text, d), maxChars)
		}
		if ch, ok := item["channel"].(map[string]any); ok {
			if id, _ := ch["id"].(string); id != "" {
				if label := channelLabel(d, id); label != id {
					ch["name"] = strings.TrimPrefix(label, "#")
				}
			}
		}
	}
	b, err = json.Marshal(items)
	if err != nil {
		return err
	}
	cols := []string{"date", "channel.name", "username", "text", "permalink"}
	if noLinks {
		cols[len(cols)-1] = "ts" // still addressable: slk get #channel <ts>
	}
	return emitList(b, cols)
}

type searchFilters struct {
	In, From, To     string
	Since, Until, On string
	Has              []string
	Threads          bool
}

// buildSearchQuery appends Slack search operators for the filters. Channel
// and user refs stay as typed (Slack search resolves names). Slack's after:
// and before: are exclusive whole days, so --since steps back a day to
// include its own date.
func buildSearchQuery(base string, f searchFilters, now time.Time) (string, error) {
	var parts []string
	if base = strings.TrimSpace(base); base != "" {
		parts = append(parts, base)
	}
	if f.In != "" {
		parts = append(parts, "in:"+strings.TrimPrefix(f.In, "#"))
	}
	if f.From != "" {
		parts = append(parts, "from:"+strings.TrimPrefix(f.From, "@"))
	}
	if f.To != "" {
		parts = append(parts, "to:"+strings.TrimPrefix(f.To, "@"))
	}
	if f.Since != "" && f.Until != "" {
		since, err1 := parsePast(f.Since, now)
		until, err2 := parsePast(f.Until, now)
		if err1 == nil && err2 == nil && until.Format("2006-01-02") <= since.Format("2006-01-02") {
			return "", fmt.Errorf("--until %s is not after --since %s: search days are whole, so the window is empty", until.Format("2006-01-02"), since.Format("2006-01-02"))
		}
	}
	for _, b := range []struct {
		value, op string
		shift     int
	}{{f.Since, "after:", -1}, {f.Until, "before:", 0}, {f.On, "on:", 0}} {
		if b.value == "" {
			continue
		}
		t, err := parsePast(b.value, now)
		if err != nil {
			return "", err
		}
		parts = append(parts, b.op+t.AddDate(0, 0, b.shift).Format("2006-01-02"))
	}
	for _, h := range f.Has {
		for _, v := range strings.Split(h, ",") {
			if v = strings.TrimSpace(v); v != "" {
				parts = append(parts, "has:"+v)
			}
		}
	}
	if f.Threads {
		parts = append(parts, "is:thread")
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("give a query or at least one filter (--in, --from, --since, …)")
	}
	return strings.Join(parts, " "), nil
}

func init() {
	searchCmd.Flags().StringVar(&searchIn, "in", "", "restrict to a channel (#name)")
	searchCmd.Flags().StringVar(&searchFrom, "from", "", "restrict to a sender (@name, or me)")
	searchCmd.Flags().StringVar(&searchTo, "to", "", "messages sent to (@name, or me: your DMs)")
	searchCmd.Flags().StringVar(&searchSince, "since", "", "on/after this day (3d, 1w, yesterday, YYYY-MM-DD)")
	searchCmd.Flags().StringVar(&searchUntil, "until", "", "before this day")
	searchCmd.Flags().StringVar(&searchOn, "on", "", "on this day")
	searchCmd.Flags().StringSliceVar(&searchHas, "has", nil, "has:link, pin, reaction, or :emoji: (repeatable)")
	searchCmd.Flags().BoolVar(&searchThreads, "thread-only", false, "only messages in threads (is:thread)")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "max matches across pages (1–10000)")
	searchCmd.Flags().StringVar(&searchOrder, "sort-dir", "desc", "timestamp order: asc or desc")
	searchCmd.Flags().BoolVar(&searchWithMeta, "with-meta", false, "with --json, wrap projected matches with total/returned/pagination metadata")
	rootCmd.AddCommand(searchCmd)
}

type searchMeta struct {
	Total               int  `json:"total"`
	Returned            int  `json:"returned"`
	HasMore             bool `json:"has_more"`
	HasNextPage         bool `json:"has_next_page"`
	PagesFetched        int  `json:"pages_fetched"`
	Page                int  `json:"page"`
	PageCount           int  `json:"page_count"`
	PageSize            int  `json:"page_size"`
	OmittedFromLastPage int  `json:"omitted_from_last_page"`
}

type searchResult struct {
	Matches    []json.RawMessage `json:"matches"`
	SearchMeta searchMeta        `json:"meta"`
}

func searchPaginationHint(meta searchMeta, limit int) {
	if !meta.HasMore {
		return
	}
	if meta.Returned == limit && limit < 10000 {
		fmt.Fprintf(stderr, "More indexed matches remain; pass --limit %d or narrow the query.\n", min(limit*2, 10000))
	} else {
		fmt.Fprintln(stderr, "Search results are partial; narrow the query to inspect remaining matches (Slack supports at most 100 pages).")
	}
}

func emitSearchEnvelope(result searchResult) error {
	// --fields selects message fields; completeness metadata remains intact.
	for i, match := range result.Matches {
		result.Matches[i] = projectOne(match, fieldsFlag)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return writeRaw(data)
}

// Keep count constant across pages: changing it shifts Slack's page offsets.
// Slack supports at most 100 results/page and 100 pages.
func fetchSearch(ctx context.Context, query string, limit int, order string) (searchResult, error) {
	result := searchResult{Matches: make([]json.RawMessage, 0)}
	if limit <= 0 || limit > 10000 {
		return result, fmt.Errorf("--limit must be between 1 and 10000 (Slack's page limit)")
	}
	if order != "asc" && order != "desc" {
		return result, fmt.Errorf("--sort-dir must be asc or desc")
	}
	count := min(limit, 100)
	for page := 1; len(result.Matches) < limit && page <= 100; page++ {
		q := url.Values{
			"query": {query}, "count": {strconv.Itoa(count)},
			"page": {strconv.Itoa(page)}, "sort": {"timestamp"}, "sort_dir": {order},
		}
		raw, err := cli.Call(ctx, "search.messages", q)
		if err != nil {
			return result, err
		}
		var env struct {
			Messages struct {
				Matches []json.RawMessage `json:"matches"`
				Total   *int              `json:"total"`
				Paging  struct {
					Pages int `json:"pages"`
					Total int `json:"total"`
				} `json:"paging"`
				Pagination struct {
					PageCount  int `json:"page_count"`
					TotalCount int `json:"total_count"`
				} `json:"pagination"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return result, err
		}
		meta := &result.SearchMeta
		meta.Total = max(env.Messages.Paging.Total, env.Messages.Pagination.TotalCount)
		if env.Messages.Total != nil {
			meta.Total = *env.Messages.Total
		}
		meta.PageCount = max(env.Messages.Paging.Pages, env.Messages.Pagination.PageCount)
		if meta.PageCount == 0 && meta.Total > 0 {
			meta.PageCount = (meta.Total + count - 1) / count
		}
		meta.Page, meta.PagesFetched, meta.PageSize = page, page, count
		meta.HasNextPage = page < meta.PageCount && page < 100
		remaining := limit - len(result.Matches)
		matches := env.Messages.Matches
		if len(matches) > remaining {
			meta.OmittedFromLastPage = len(matches) - remaining
			matches = matches[:remaining]
		}
		result.Matches = append(result.Matches, matches...)
		meta.Returned = len(result.Matches)
		meta.HasMore = meta.Total > meta.Returned || meta.HasNextPage || meta.OmittedFromLastPage > 0
		if len(matches) == 0 || !meta.HasNextPage {
			break
		}
	}
	return result, nil
}
