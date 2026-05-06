package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"symphony-go/internal/runtime/observability"
)

type Options struct {
	MaxAgents   int
	ProjectSlug string
	Color       bool
}

func Render(snapshot observability.Snapshot, opts Options) string {
	now := snapshot.GeneratedAt
	if now.IsZero() {
		now = time.Now()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "╭─ %s\n", colorize(opts, "SYMPHONY STATUS", ansiBold+ansiCyan))
	fmt.Fprintf(&b, "│ Agents: %s/%s\n", formatInt(snapshot.Counts.Running), formatInt(maxAgents(opts.MaxAgents)))
	fmt.Fprintln(&b, "│ Throughput: 0 tps")
	fmt.Fprintf(&b, "│ Runtime: %s\n", formatRuntimeSeconds(snapshot.TotalRuntimeSeconds(now)))
	fmt.Fprintf(&b, "│ Tokens: in %s | out %s | total %s\n",
		formatInt(snapshot.CodexTotals.InputTokens),
		formatInt(snapshot.CodexTotals.OutputTokens),
		formatInt(snapshot.CodexTotals.TotalTokens),
	)
	fmt.Fprintf(&b, "│ Rate Limits: %s\n", formatRateLimits(snapshot.RateLimits))
	fmt.Fprintf(&b, "│ Project: %s\n", projectURL(opts.ProjectSlug))
	fmt.Fprintf(&b, "│ Next refresh: %s\n", formatNextRefresh(snapshot.Polling, now))
	if snapshot.LastError != "" {
		fmt.Fprintf(&b, "│ %s %s\n", colorize(opts, "Last error:", ansiBold+ansiRed), truncate(snapshot.LastError, 120))
	}

	fmt.Fprintf(&b, "├─ %s\n", colorize(opts, "Running", ansiBold+ansiGreen))
	fmt.Fprintln(&b, "│")
	fmt.Fprintln(&b, "│   ID       STATE          PHASE/STAGE           PID      AGE / TURN   TOKENS     SESSION        EVENT")
	fmt.Fprintln(&b, "│   ─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────")
	running := snapshot.Running
	if opts.MaxAgents > 0 && len(running) > opts.MaxAgents {
		running = running[:opts.MaxAgents]
	}
	if len(running) == 0 {
		fmt.Fprintln(&b, "│  No active agents")
	} else {
		for _, entry := range running {
			fmt.Fprintf(&b, "│ %s %-8s %-14s %-21s %-8s %-12s %8s %-14s %s\n",
				colorize(opts, "●", ansiGreen),
				valueOrDefault(entry.IssueIdentifier, entry.IssueID),
				valueOrDefault(entry.State, "-"),
				stageLabel(entry),
				formatPID(entry.PID),
				fmt.Sprintf("%s / %d", formatAge(entry.StartedAt, now), entry.TurnCount),
				formatInt(entry.Tokens.TotalTokens),
				compactSessionID(entry.SessionID),
				truncate(lastActivity(entry), 64),
			)
		}
		if opts.MaxAgents > 0 && len(snapshot.Running) > opts.MaxAgents {
			fmt.Fprintf(&b, "│  ... %d more active agents\n", len(snapshot.Running)-opts.MaxAgents)
		}
	}

	fmt.Fprintln(&b, "│")
	fmt.Fprintf(&b, "├─ %s\n", colorize(opts, "Backoff queue", ansiBold+ansiYellow))
	fmt.Fprintln(&b, "│")
	if len(snapshot.Retrying) == 0 {
		fmt.Fprintln(&b, "│  No queued retries")
	} else {
		for _, entry := range snapshot.Retrying {
			fmt.Fprintf(&b, "│  %s %s attempt=%d %s error=%s\n",
				colorize(opts, "↻", ansiYellow),
				valueOrDefault(entry.IssueIdentifier, entry.IssueID),
				entry.Attempt,
				formatDue(entry.DueAt, now),
				truncate(valueOrDefault(entry.Error, "-"), 100),
			)
		}
	}
	fmt.Fprintln(&b, "╰─")

	return strings.TrimRight(b.String(), "\n")
}

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
)

func colorize(opts Options, text, code string) string {
	if !opts.Color || text == "" {
		return text
	}
	return code + text + ansiReset
}

func ClearAndRender(frame string) string {
	return "\033[H\033[2J" + frame + "\n"
}

func compactSessionID(sessionID string) string {
	if sessionID == "" {
		return "-"
	}
	if len(sessionID) <= 12 {
		return sessionID
	}
	return sessionID[:4] + "..." + sessionID[len(sessionID)-6:]
}

func lastActivity(entry observability.RunningEntry) string {
	if entry.LastMessage != "" {
		return entry.LastMessage
	}
	if entry.LastEvent != "" {
		return entry.LastEvent
	}
	return "-"
}

func stageLabel(entry observability.RunningEntry) string {
	if entry.AgentPhase != "" && entry.Stage != "" {
		return entry.AgentPhase + "/" + entry.Stage
	}
	if entry.Stage != "" {
		return entry.Stage
	}
	return "-"
}

func formatAge(startedAt, now time.Time) string {
	if startedAt.IsZero() {
		return "-"
	}
	return formatDuration(now.Sub(startedAt))
}

func formatDue(dueAt, now time.Time) string {
	if dueAt.IsZero() {
		return "-"
	}
	if dueAt.After(now) {
		return "in " + formatDuration(dueAt.Sub(now))
	}
	return formatDuration(now.Sub(dueAt)) + " ago"
}

func formatNextRefresh(polling observability.PollingStatus, now time.Time) string {
	if polling.Checking {
		return "checking now…"
	}
	if polling.NextPollInMS > 0 {
		seconds := (polling.NextPollInMS + 999) / 1000
		return fmt.Sprintf("%ds", seconds)
	}
	if !polling.NextPollAt.IsZero() {
		return formatDue(polling.NextPollAt, now)
	}
	return "n/a"
}

func projectURL(slug string) string {
	if slug == "" {
		return "n/a"
	}
	return "https://linear.app/project/" + slug + "/issues"
}

func formatRuntimeSeconds(seconds float64) string {
	if seconds <= 0 {
		return "0m 0s"
	}
	duration := time.Duration(seconds) * time.Second
	hours := int(duration / time.Hour)
	minutes := int((duration % time.Hour) / time.Minute)
	secs := int((duration % time.Minute) / time.Second)
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, secs)
	}
	return fmt.Sprintf("%dm %ds", minutes, secs)
}

func formatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = -duration
	}
	duration = duration.Round(time.Second)
	if duration < time.Minute {
		return fmt.Sprintf("%.3fs", duration.Seconds())
	}
	if duration < time.Hour {
		minutes := int(duration / time.Minute)
		seconds := int((duration % time.Minute) / time.Second)
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := int(duration / time.Hour)
	minutes := int((duration % time.Hour) / time.Minute)
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func formatRateLimits(rateLimits any) string {
	if rateLimits == nil {
		return "unavailable"
	}
	values, ok := rateLimits.(map[string]any)
	if !ok {
		return fmt.Sprint(rateLimits)
	}

	var parts []string
	if reached := stringValue(values["rateLimitReachedType"]); reached != "" {
		parts = append(parts, "limited: "+reached)
	} else {
		parts = append(parts, "not limited")
	}
	for _, name := range []string{"primary", "secondary"} {
		bucket, ok := values[name].(map[string]any)
		if !ok {
			continue
		}
		used, ok := numberText(bucket["usedPercent"])
		if !ok {
			continue
		}
		part := fmt.Sprintf("%s %s%% used", name, used)
		if window, ok := numberText(bucket["windowDurationMins"]); ok {
			part += " / " + window + "m window"
		}
		parts = append(parts, part)
	}
	if remaining, ok := numberText(values["remaining"]); ok {
		parts = append(parts, "remaining "+remaining)
	}
	return strings.Join(parts, " | ")
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" || text == "null" {
		return ""
	}
	return text
}

func numberText(value any) (string, bool) {
	switch typed := value.(type) {
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case float64:
		if math.Trunc(typed) == typed {
			return strconv.FormatInt(int64(typed), 10), true
		}
		return strconv.FormatFloat(typed, 'f', 1, 64), true
	case float32:
		return numberText(float64(typed))
	default:
		return "", false
	}
}

func formatInt(value int) string {
	return fmt.Sprintf("%d", value)
}

func maxAgents(value int) int {
	if value > 0 {
		return value
	}
	return 1
}

func formatPID(pid int) string {
	if pid <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d", pid)
}

func valueOrDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
