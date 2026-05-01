package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Logger struct {
	mu           sync.Mutex
	file         *os.File
	encoder      *json.Encoder
	human        *os.File
	humanPath    string
	humanColor   bool
	humanMin     slog.Level
	humanIssue   string
	console      io.Writer
	color        bool
	min          slog.Level
	consoleIssue string
}

type Event struct {
	Time            string         `json:"time"`
	Level           string         `json:"level,omitempty"`
	Issue           string         `json:"issue,omitempty"`
	IssueID         string         `json:"issue_id,omitempty"`
	IssueIdentifier string         `json:"issue_identifier,omitempty"`
	SessionID       string         `json:"session_id,omitempty"`
	Event           string         `json:"event"`
	Message         string         `json:"message,omitempty"`
	Fields          map[string]any `json:"fields,omitempty"`
}

type Option func(*Logger)

func WithConsole(writer io.Writer, color bool) Option {
	return func(l *Logger) {
		l.console = writer
		l.color = color
		l.min = slog.LevelInfo
	}
}

func WithConsoleMinLevel(level slog.Level) Option {
	return func(l *Logger) {
		l.min = level
	}
}

func WithHumanFile(path string, color bool) Option {
	return func(l *Logger) {
		l.humanPath = path
		l.humanColor = color
		l.humanMin = slog.LevelInfo
	}
}

func WithHumanFileMinLevel(level slog.Level) Option {
	return func(l *Logger) {
		l.humanMin = level
	}
}

func New(path string, options ...Option) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	logger := &Logger{file: file, encoder: json.NewEncoder(file)}
	for _, option := range options {
		option(logger)
	}
	if logger.humanPath != "" {
		if err := os.MkdirAll(filepath.Dir(logger.humanPath), 0o755); err != nil {
			_ = file.Close()
			return nil, err
		}
		human, err := os.OpenFile(logger.humanPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		logger.human = human
	}
	return logger, nil
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	var firstErr error
	if l.file != nil {
		firstErr = l.file.Close()
	}
	if l.human != nil {
		if err := l.human.Close(); firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (l *Logger) Write(event Event) error {
	if l == nil {
		return nil
	}
	if event.Time == "" {
		event.Time = time.Now().Format(time.RFC3339Nano)
	}
	if event.Level == "" {
		event.Level = LevelName(InferLevel(event))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.encoder.Encode(event); err != nil {
		return err
	}
	displayEvent, ok := HumanEvent(event)
	if !ok {
		return nil
	}
	level := parseLevel(displayEvent.Level)
	if l.human != nil && level >= l.humanMin {
		if err := l.writeDisplay(l.human, displayEvent, l.humanColor, l.humanMin, &l.humanIssue); err != nil {
			return err
		}
	}
	if l.console != nil && level >= l.min {
		return l.writeDisplay(l.console, displayEvent, l.color, l.min, &l.consoleIssue)
	}
	return nil
}

func (l *Logger) writeDisplay(writer io.Writer, event Event, color bool, min slog.Level, lastIssue *string) error {
	issue := displayIssue(event)
	if issue != "" && issue != *lastIssue {
		section := issueSectionEvent(event, issue)
		if parseLevel(section.Level) >= min {
			if _, err := fmt.Fprintln(writer, FormatConsole(section, color)); err != nil {
				return err
			}
		}
		*lastIssue = issue
	}
	_, err := fmt.Fprintln(writer, FormatConsole(event, color))
	return err
}

func LogPath(baseDir string) string {
	name := fmt.Sprintf("run-%s.jsonl", time.Now().Format("20060102-150405"))
	return filepath.Join(baseDir, ".symphony", "logs", name)
}

func HumanLogPath(jsonlPath string) string {
	return strings.TrimSuffix(jsonlPath, filepath.Ext(jsonlPath)) + ".human.log"
}

func displayIssue(event Event) string {
	return firstNonEmpty(event.IssueIdentifier, event.Issue)
}

func issueSectionEvent(event Event, issue string) Event {
	return Event{
		Time:            event.Time,
		Level:           "info",
		Issue:           event.Issue,
		IssueID:         event.IssueID,
		IssueIdentifier: firstNonEmpty(event.IssueIdentifier, issue),
		Event:           "issue_section",
		Message:         "Issue " + issue,
	}
}

func InferLevel(event Event) slog.Level {
	switch event.Event {
	case "poll_error", "reconcile_error", "reconcile_refresh_failed", "startup_cleanup_fetch_failed",
		"issue_error", "retry_fetch_error", "workspace_cleanup_failed", "after_run_hook_failed",
		"workflow_reload_failed":
		return slog.LevelError
	case "dispatch_skipped", "waiting_for_review", "waiting_for_ai_review", "worker_stalled",
		"ai_review_failed":
		return slog.LevelWarn
	case "codex_event", "workspace_cleaned":
		return slog.LevelDebug
	}
	name := strings.ToLower(event.Event)
	if strings.Contains(name, "blocked") {
		return slog.LevelWarn
	}
	if strings.Contains(name, "failed") || strings.Contains(name, "error") {
		return slog.LevelError
	}
	return slog.LevelInfo
}

func LevelName(level slog.Level) string {
	return strings.ToLower(level.String())
}

func HumanEvent(event Event) (Event, bool) {
	if event.Event == "workspace_cleaned" {
		return Event{}, false
	}
	if isSuccessfulStartupCleanupHook(event) {
		return Event{}, false
	}
	if event.Event != "codex_event" {
		return event, true
	}
	return humanCodexEvent(event)
}

func isSuccessfulStartupCleanupHook(event Event) bool {
	if !strings.HasPrefix(event.Event, "workspace_hook_") {
		return false
	}
	if event.Event == "workspace_hook_failed" {
		return false
	}
	if stringField(event.Fields, "source") != "startup_cleanup" {
		return false
	}
	stage := stringField(event.Fields, "stage")
	return stage == "started" || stage == "completed"
}

func humanCodexEvent(event Event) (Event, bool) {
	method := firstNonEmpty(event.Message, stringField(event.Fields, "method"))
	params := mapField(event.Fields, "params")
	display := Event{
		Time:            event.Time,
		Level:           "info",
		Issue:           event.Issue,
		IssueID:         event.IssueID,
		IssueIdentifier: event.IssueIdentifier,
		SessionID:       codexSessionID(event, params),
	}

	switch method {
	case "session_started":
		display.Event = "codex_session_started"
		display.Message = "Codex session started"
		display.Fields = compactMap(map[string]any{
			"pid": intField(event.Fields, "pid"),
		})
		return display, true
	case "turn_started":
		display.Event = "codex_turn_started"
		display.Message = "Codex turn started"
		display.Fields = compactMap(map[string]any{
			"turn":         intField(event.Fields, "turn_count"),
			"continuation": event.Fields["continuation"],
			"pid":          intField(event.Fields, "pid"),
		})
		return display, true
	case "turn_completed":
		display.Event = "codex_turn_completed"
		display.Message = "Codex turn completed"
		display.Fields = compactMap(map[string]any{
			"turn":         intField(event.Fields, "turn_count"),
			"continuation": event.Fields["continuation"],
		})
		return display, true
	case "turn/plan/updated":
		display.Event = "codex_plan"
		display.Level = "debug"
		display.Message = "Codex plan updated"
		display.Fields = planFields(params)
		return display, true
	case "item/completed":
		return humanCompletedItem(event, display, params)
	case "turn/diff/updated":
		display.Event = "codex_diff"
		display.Level = "debug"
		display.Message = "Codex diff updated"
		display.Fields = diffFields(stringField(params, "diff"))
		if len(display.Fields) == 0 {
			return Event{}, false
		}
		return display, true
	case "thread/tokenUsage/updated":
		return Event{}, false
	default:
		return Event{}, false
	}
}

func humanCompletedItem(_ Event, display Event, params map[string]any) (Event, bool) {
	item := mapField(params, "item")
	itemType := stringField(item, "type")
	switch itemType {
	case "agentMessage":
		text := previewText(stringField(item, "text"), 220)
		if text == "" {
			return Event{}, false
		}
		phase := stringField(item, "phase")
		display.Event = "codex_message"
		display.Level = "debug"
		display.Message = text
		if phase == "final_answer" {
			display.Event = "codex_final"
			display.Level = "info"
		}
		display.Fields = compactMap(map[string]any{"phase": phase})
		return display, true
	case "commandExecution":
		exitCode := intField(item, "exitCode")
		command := commandLabel(item)
		cwd := stringField(item, "cwd")
		display.Event = "codex_command"
		display.Message = "Codex command completed"
		if isContextReadCommand(command, cwd) {
			display.Level = "debug"
		}
		if exitCode != nil && *exitCode != 0 {
			display.Level = "warn"
		}
		display.Fields = compactMap(map[string]any{
			"command":     command,
			"cwd":         basePath(cwd),
			"duration_ms": intField(item, "durationMs"),
			"exit_code":   exitCode,
			"output":      previewText(stringField(item, "aggregatedOutput"), 180),
		})
		return display, true
	case "fileChange":
		display.Event = "codex_file_change"
		display.Message = "Codex file change completed"
		display.Fields = fileChangeFields(item)
		return display, true
	default:
		return Event{}, false
	}
}

func planFields(params map[string]any) map[string]any {
	plan, _ := params["plan"].([]any)
	total := len(plan)
	completed := 0
	current := ""
	next := ""
	for _, entry := range plan {
		item := mapFromAny(entry)
		status := stringField(item, "status")
		step := previewText(stringField(item, "step"), 140)
		switch status {
		case "completed":
			completed++
		case "inProgress":
			if current == "" {
				current = step
			}
		case "pending":
			if next == "" {
				next = step
			}
		}
	}
	progress := ""
	if total > 0 {
		progress = fmt.Sprintf("%d/%d", completed, total)
	}
	return compactMap(map[string]any{
		"progress":    progress,
		"current":     current,
		"next":        next,
		"explanation": previewText(stringField(params, "explanation"), 180),
	})
}

func fileChangeFields(item map[string]any) map[string]any {
	changes, _ := item["changes"].([]any)
	paths := make([]string, 0, len(changes))
	added := 0
	removed := 0
	for _, change := range changes {
		entry := mapFromAny(change)
		if path := basePath(stringField(entry, "path")); path != "" {
			paths = append(paths, path)
		}
		plus, minus := diffLineCounts(stringField(entry, "diff"))
		added += plus
		removed += minus
	}
	return compactMap(map[string]any{
		"files":   strings.Join(paths, ","),
		"summary": diffSummary(added, removed),
	})
}

func diffFields(diff string) map[string]any {
	if strings.TrimSpace(diff) == "" {
		return nil
	}
	paths := make([]string, 0)
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "diff --git ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			paths = append(paths, strings.TrimPrefix(parts[3], "b/"))
		}
	}
	added, removed := diffLineCounts(diff)
	return compactMap(map[string]any{
		"files":   strings.Join(paths, ","),
		"summary": diffSummary(added, removed),
	})
}

func diffLineCounts(diff string) (int, int) {
	added := 0
	removed := 0
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return added, removed
}

func diffSummary(added, removed int) string {
	if added == 0 && removed == 0 {
		return ""
	}
	return fmt.Sprintf("+%d/-%d", added, removed)
}

func commandLabel(item map[string]any) string {
	actions, _ := item["commandActions"].([]any)
	for _, action := range actions {
		if command := stringField(mapFromAny(action), "command"); command != "" {
			return previewText(command, 180)
		}
	}
	return previewText(stripShellWrapper(stringField(item, "command")), 180)
}

func isContextReadCommand(command, cwd string) bool {
	value := command + " " + cwd
	for _, marker := range []string{
		"/.codex/memories",
		"/.codex/memories_extensions",
		".codex/skills",
		"MEMORY.md",
		"memory_summary.md",
		"raw_memories.md",
		"rollout_summaries/",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func stripShellWrapper(command string) string {
	command = strings.TrimSpace(command)
	for _, prefix := range []string{"/bin/zsh -lc ", "bash -lc ", "/bin/bash -lc "} {
		if strings.HasPrefix(command, prefix) {
			command = strings.TrimSpace(strings.TrimPrefix(command, prefix))
			break
		}
	}
	if len(command) >= 2 {
		if (command[0] == '\'' && command[len(command)-1] == '\'') || (command[0] == '"' && command[len(command)-1] == '"') {
			command = command[1 : len(command)-1]
		}
	}
	return command
}

func codexSessionID(event Event, params map[string]any) string {
	if event.SessionID != "" {
		return event.SessionID
	}
	if sessionID := stringField(event.Fields, "session_id"); sessionID != "" {
		return sessionID
	}
	threadID := firstNonEmpty(stringField(event.Fields, "thread_id"), stringField(params, "threadId"))
	turnID := firstNonEmpty(stringField(event.Fields, "turn_id"), stringField(params, "turnId"))
	if threadID != "" && turnID != "" {
		return threadID + "-" + turnID
	}
	return threadID
}

func mapField(fields map[string]any, key string) map[string]any {
	if fields == nil {
		return nil
	}
	return mapFromAny(fields[key])
}

func mapFromAny(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func stringField(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	value, ok := fields[key]
	if !ok || value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	return fmt.Sprint(value)
}

func intField(fields map[string]any, key string) *int {
	if fields == nil {
		return nil
	}
	switch value := fields[key].(type) {
	case int:
		return &value
	case int64:
		converted := int(value)
		return &converted
	case float64:
		converted := int(value)
		return &converted
	case json.Number:
		parsed, err := strconv.Atoi(value.String())
		if err != nil {
			return nil
		}
		return &parsed
	default:
		return nil
	}
}

func compactMap(fields map[string]any) map[string]any {
	compacted := map[string]any{}
	for key, value := range fields {
		switch typed := value.(type) {
		case nil:
			continue
		case *int:
			if typed == nil {
				continue
			}
			compacted[key] = *typed
		case string:
			if strings.TrimSpace(typed) == "" {
				continue
			}
			compacted[key] = typed
		default:
			compacted[key] = value
		}
	}
	if len(compacted) == 0 {
		return nil
	}
	return compacted
}

func previewText(value string, max int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	return truncateRunes(value, max)
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func basePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

func FormatConsole(event Event, color bool) string {
	level := parseLevel(event.Level)
	levelText := strings.ToUpper(LevelName(level))
	if color {
		levelText = colorizeLevel(level, levelText)
	}

	parts := []string{
		formatEventTime(event.Time),
		fmt.Sprintf("%-5s", levelText),
		"event=" + safeValue(event.Event),
	}
	if issue := firstNonEmpty(event.IssueIdentifier, event.Issue); issue != "" {
		parts = append(parts, "issue="+safeValue(issue))
	}
	if event.SessionID != "" {
		parts = append(parts, "session="+safeValue(compact(event.SessionID, 14)))
	}
	parts = append(parts, compactFields(event.Fields)...)
	if event.Message != "" {
		parts = append(parts, safeMessage(event.Message))
	}
	return strings.Join(parts, " ")
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func formatEventTime(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.IsZero() {
		return "--:--:--"
	}
	return parsed.Format("15:04:05")
}

func compactFields(fields map[string]any) []string {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		if compactFieldAllowed(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := compactFieldValue(fields[key])
		if value == "" {
			continue
		}
		parts = append(parts, key+"="+safeValue(value))
	}
	return parts
}

func compactFieldAllowed(key string) bool {
	switch key {
	case "issue_id", "issue_identifier", "params", "session_id", "thread_id", "turn_id":
		return false
	default:
		return true
	}
}

func compactFieldValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []string:
		return strings.Join(typed, ",")
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, fmt.Sprint(item))
		}
		return strings.Join(values, ",")
	default:
		return fmt.Sprint(typed)
	}
}

func safeMessage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return "msg=" + safeValue(value)
}

func safeValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", "\\n")
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t=") {
		return strconv.Quote(value)
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compact(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	keep := max - 3
	left := keep / 2
	right := keep - left
	return value[:left] + "..." + value[len(value)-right:]
}

func colorizeLevel(level slog.Level, text string) string {
	const (
		reset  = "\033[0m"
		dim    = "\033[2m"
		cyan   = "\033[36m"
		yellow = "\033[33m"
		red    = "\033[31m"
	)
	switch level {
	case slog.LevelDebug:
		return dim + text + reset
	case slog.LevelWarn:
		return yellow + text + reset
	case slog.LevelError:
		return red + text + reset
	default:
		return cyan + text + reset
	}
}
