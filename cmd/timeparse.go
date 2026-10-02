package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// reRelative matches "30m", "2h", "3d", "1w" with an optional "+" or "in ".
var reRelative = regexp.MustCompile(`^(?:\+|in\s+)?(\d+)\s*(m|min|h|d|w)$`)

var relativeUnits = map[string]time.Duration{
	"m": time.Minute, "min": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour,
}

// localDayLayouts are absolute forms read in local time.
var localDayLayouts = []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02"}

// parsePast reads a point in the past: 30m/2h/3d/1w ago, now, today,
// yesterday (local midnight), YYYY-MM-DD[ HH:MM] local, RFC3339, epoch.
func parsePast(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if m := reRelative.FindStringSubmatch(s); m != nil && !strings.HasPrefix(s, "+") && !strings.HasPrefix(s, "in") {
		n, _ := strconv.Atoi(m[1])
		return now.Add(-time.Duration(n) * relativeUnits[m[2]]), nil
	}
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch s {
	case "now":
		return now, nil
	case "today":
		return midnight, nil
	case "yesterday":
		return midnight.AddDate(0, 0, -1), nil
	}
	return parseAbsolute(s, now.Location())
}

// parseFuture reads a point in the future for scheduling: +2h / in 30m,
// HH:MM (next occurrence), tomorrow [HH:MM], YYYY-MM-DD HH:MM local, RFC3339,
// epoch.
func parseFuture(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if m := reRelative.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return now.Add(time.Duration(n) * relativeUnits[m[2]]), nil
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	clock := s
	tomorrow := false
	if rest, ok := strings.CutPrefix(s, "tomorrow"); ok {
		tomorrow, clock = true, strings.TrimSpace(rest)
		if clock == "" {
			clock = "09:00"
		}
	}
	if c, err := time.ParseInLocation("15:04", clock, now.Location()); err == nil {
		t := day.Add(time.Duration(c.Hour())*time.Hour + time.Duration(c.Minute())*time.Minute)
		if tomorrow || !t.After(now) {
			t = t.AddDate(0, 0, 1)
		}
		return t, nil
	}
	if tomorrow {
		return time.Time{}, fmt.Errorf("cannot parse time %q: use 'tomorrow HH:MM'", s)
	}
	return parseAbsolute(s, now.Location())
}

func parseAbsolute(s string, loc *time.Location) (time.Time, error) {
	if sec, frac, ok := strings.Cut(s, "."); ok && reTS.MatchString(s) {
		n, _ := strconv.ParseInt(sec, 10, 64)
		ns, _ := strconv.ParseInt(frac, 10, 64)
		return time.Unix(n, ns*1000), nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(n, 0), nil
	}
	if t, err := time.Parse(time.RFC3339, strings.ToUpper(s)); err == nil {
		return t, nil
	}
	for _, layout := range localDayLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q: use 2h/3d/1w, today, yesterday, YYYY-MM-DD[ HH:MM], RFC3339, epoch, or a ts", s)
}

// parseTimeBound turns a --since/--until value into what Slack's
// oldest/latest expects: a ts passes through unchanged, anything else becomes
// epoch seconds.
func parseTimeBound(s string) (string, error) {
	if reTS.MatchString(s) {
		return s, nil
	}
	t, err := parsePast(s, time.Now())
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(t.Unix(), 10), nil
}

// tsOf formats t as a Slack ts.
func tsOf(t time.Time) string {
	return fmt.Sprintf("%d.%06d", t.Unix(), t.Nanosecond()/1000)
}
