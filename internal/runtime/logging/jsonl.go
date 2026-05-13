package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
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
	visibleIssue string
	console      io.Writer
	color        bool
	min          slog.Level
	consoleIssue string
	issueBaseDir string
	issueColor   bool
	issueMin     slog.Level
	issueSinks   map[string]*issueSink
}

type issueSink struct {
	file       *os.File
	encoder    *json.Encoder
	human      *os.File
	humanIssue string
}

type Event struct {
	Time            string         `json:"time"`
	Level           string         `json:"level,omitempty"`
	Issue           string         `json:"issue,omitempty"`
	IssueID         string         `json:"issue_id,omitempty"`
	IssueIdentifier string         `json:"issue_identifier,omitempty"`
	SessionID       string         `json:"session_id,omitempty"`
	TraceID         string         `json:"trace_id,omitempty"`
	SpanID          string         `json:"span_id,omitempty"`
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

func WithIssueFiles(baseDir string, color bool) Option {
	return func(l *Logger) {
		l.issueBaseDir = baseDir
		l.issueColor = color
		l.issueMin = slog.LevelInfo
	}
}

func WithIssueFilesMinLevel(level slog.Level) Option {
	return func(l *Logger) {
		l.issueMin = level
	}
}

func WithVisibleIssueFilter(issue string) Option {
	return func(l *Logger) {
		l.visibleIssue = strings.TrimSpace(issue)
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
	logger := &Logger{file: file, encoder: json.NewEncoder(file), issueSinks: map[string]*issueSink{}}
	for _, option := range options {
		option(logger)
	}
	if logger.humanPath != "" {
		if err := os.MkdirAll(filepath.Dir(logger.humanPath), 0o755); err != nil {
			logger.warnSinkFailed("human_file", logger.humanPath, err)
		} else if human, err := os.OpenFile(logger.humanPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err != nil {
			logger.warnSinkFailed("human_file", logger.humanPath, err)
		} else {
			logger.human = human
		}
	}
	return logger, nil
}

func (l *Logger) warnSinkFailed(sink, path string, failure error) {
	_ = l.Write(Event{
		Level:   "warn",
		Event:   "log_sink_failed",
		Message: "configured log sink disabled",
		Fields: map[string]any{
			"sink":  sink,
			"path":  path,
			"error": failure.Error(),
		},
	})
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
	for _, sink := range l.issueSinks {
		if sink.file != nil {
			if err := sink.file.Close(); firstErr == nil {
				firstErr = err
			}
		}
		if sink.human != nil {
			if err := sink.human.Close(); firstErr == nil {
				firstErr = err
			}
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
	if err := l.writeIssueFile(event); err != nil {
		return err
	}
	displayEvent, ok := HumanEvent(event)
	if !ok {
		return nil
	}
	if !visibleIssueAllowed(displayEvent, l.visibleIssue) {
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

func (l *Logger) writeIssueFile(event Event) error {
	issue := displayIssue(event)
	if issue == "" || l.issueBaseDir == "" {
		return nil
	}
	sink, err := l.issueSink(issue)
	if err != nil {
		l.warnSinkFailedLocked("issue_file", IssueLogPath(l.issueBaseDir, issue), err)
		return nil
	}
	if err := sink.encoder.Encode(event); err != nil {
		return err
	}
	displayEvent, ok := HumanEvent(event)
	if !ok {
		return nil
	}
	level := parseLevel(displayEvent.Level)
	if sink.human != nil && level >= l.issueMin {
		return l.writeDisplay(sink.human, displayEvent, l.issueColor, l.issueMin, &sink.humanIssue)
	}
	return nil
}

func (l *Logger) issueSink(issue string) (*issueSink, error) {
	name := safeLogName(issue)
	if name == "" {
		return nil, fmt.Errorf("issue identifier is empty after sanitization")
	}
	if sink, ok := l.issueSinks[name]; ok {
		return sink, nil
	}
	path := filepath.Join(l.issueBaseDir, ".symphony", "logs", name+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	humanPath := HumanLogPath(path)
	human, err := os.OpenFile(humanPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	sink := &issueSink{file: file, encoder: json.NewEncoder(file), human: human}
	l.issueSinks[name] = sink
	return sink, nil
}

func (l *Logger) warnSinkFailedLocked(sink, path string, failure error) {
	_ = l.encoder.Encode(Event{
		Time:    time.Now().Format(time.RFC3339Nano),
		Level:   "warn",
		Event:   "log_sink_failed",
		Message: "configured log sink disabled",
		Fields: map[string]any{
			"sink":  sink,
			"path":  path,
			"error": failure.Error(),
		},
	})
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

func IssueLogPath(baseDir string, issueIdentifier string) string {
	name := safeLogName(issueIdentifier)
	if name == "" {
		return LogPath(baseDir)
	}
	return filepath.Join(baseDir, ".symphony", "logs", name+".jsonl")
}

func HumanLogPath(jsonlPath string) string {
	return strings.TrimSuffix(jsonlPath, filepath.Ext(jsonlPath)) + ".human.log"
}

func safeLogName(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	return strings.Trim(builder.String(), "-_.")
}

func displayIssue(event Event) string {
	return firstNonEmpty(event.IssueIdentifier, event.Issue)
}

func visibleIssueAllowed(event Event, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	if event.IssueIdentifier == "" && event.Issue == "" && event.IssueID == "" {
		return true
	}
	return event.IssueIdentifier == filter || event.Issue == filter || event.IssueID == filter
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
		rawText := stringField(item, "text")
		text := previewText(rawText, 220)
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
		fields := compactMap(map[string]any{"phase": phase})
		for key, value := range evidenceFieldsFromText(rawText) {
			fields[key] = value
		}
		display.Fields = fields
		return display, true
	case "commandExecution":
		exitCode := intField(item, "exitCode")
		command := commandLabel(item)
		cwd := stringField(item, "cwd")
		durationMS := intField(item, "durationMs")
		display.Event = "codex_command"
		display.Message = commandMessage(command, exitCode, durationMS)
		if isContextReadCommand(command, cwd) {
			display.Level = "debug"
		}
		if exitCode != nil && *exitCode != 0 {
			display.Level = "warn"
		}
		display.Fields = compactMap(map[string]any{
			"command":        command,
			"command_kind":   commandKind(command),
			"command_status": commandStatus(exitCode),
			"cwd":            basePath(cwd),
			"duration_ms":    durationMS,
			"exit_code":      exitCode,
			"output":         previewText(stringField(item, "aggregatedOutput"), 180),
		})
		return display, true
	case "fileChange":
		display.Event = "codex_file_change"
		display.Fields = fileChangeFields(item)
		display.Message = fileChangeMessage(display.Fields)
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
	locations := make([]string, 0, len(changes))
	added := 0
	removed := 0
	lineStart := 0
	lineEnd := 0
	for _, change := range changes {
		entry := mapFromAny(change)
		path := basePath(stringField(entry, "path"))
		if path != "" {
			paths = append(paths, path)
		}
		diff := stringField(entry, "diff")
		plus, minus := diffLineCounts(diff)
		added += plus
		removed += minus
		start, end := diffLineRange(diff)
		if path != "" && start > 0 {
			locations = append(locations, lineLocation(path, start, end))
		}
		if start > 0 && (lineStart == 0 || start < lineStart) {
			lineStart = start
		}
		if end > lineEnd {
			lineEnd = end
		}
	}
	return compactMap(map[string]any{
		"additions":          added,
		"changed_lines":      added + removed,
		"deletions":          removed,
		"evidence_file":      firstString(paths),
		"evidence_line":      intIfPositive(lineStart),
		"evidence_locations": strings.Join(locations, ","),
		"file":               firstString(paths),
		"file_count":         intIfPositive(len(paths)),
		"file_locations":     strings.Join(locations, ","),
		"files":              strings.Join(paths, ","),
		"line_end":           intIfPositive(lineEnd),
		"line_start":         intIfPositive(lineStart),
		"summary":            diffSummary(added, removed),
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

func diffLineRange(diff string) (int, int) {
	start := 0
	end := 0
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "@@ ") {
			continue
		}
		lineStart, lineEnd, ok := parseHunkLineRange(line)
		if !ok {
			continue
		}
		if start == 0 || lineStart < start {
			start = lineStart
		}
		if lineEnd > end {
			end = lineEnd
		}
	}
	return start, end
}

func parseHunkLineRange(line string) (int, int, bool) {
	fields := strings.Fields(line)
	oldStart, oldCount, oldOK := 0, 0, false
	for _, field := range fields {
		switch {
		case strings.HasPrefix(field, "+"):
			start, count, ok := parseRangeToken(field)
			if !ok {
				continue
			}
			if count <= 0 && oldOK {
				return oldStart, oldStart + maxInt(oldCount, 1) - 1, true
			}
			return start, start + maxInt(count, 1) - 1, true
		case strings.HasPrefix(field, "-"):
			oldStart, oldCount, oldOK = parseRangeToken(field)
		}
	}
	return 0, 0, false
}

func parseRangeToken(token string) (int, int, bool) {
	token = strings.TrimLeft(token, "+-")
	parts := strings.SplitN(token, ",", 2)
	start, err := strconv.Atoi(parts[0])
	if err != nil || start < 0 {
		return 0, 0, false
	}
	count := 1
	if len(parts) == 2 {
		parsed, err := strconv.Atoi(parts[1])
		if err != nil || parsed < 0 {
			return 0, 0, false
		}
		count = parsed
	}
	return start, count, true
}

func diffSummary(added, removed int) string {
	if added == 0 && removed == 0 {
		return ""
	}
	return fmt.Sprintf("+%d/-%d", added, removed)
}

func lineLocation(path string, start, end int) string {
	if start <= 0 {
		return path
	}
	if end <= start {
		return fmt.Sprintf("%s:%d", path, start)
	}
	return fmt.Sprintf("%s:%d-%d", path, start, end)
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func intIfPositive(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
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

func commandStatus(exitCode *int) string {
	if exitCode != nil && *exitCode != 0 {
		return "failed"
	}
	return "succeeded"
}

func commandMessage(command string, exitCode, durationMS *int) string {
	status := commandStatus(exitCode)
	if command == "" {
		command = "command"
	}
	detail := ""
	if exitCode != nil && *exitCode != 0 {
		detail = fmt.Sprintf("exit=%d", *exitCode)
	}
	if durationMS != nil {
		if detail != "" {
			detail += " "
		}
		detail += fmt.Sprintf("%dms", *durationMS)
	}
	if detail != "" {
		return fmt.Sprintf("Command %s: %s (%s)", status, command, detail)
	}
	return fmt.Sprintf("Command %s: %s", status, command)
}

func commandKind(command string) string {
	command = strings.TrimSpace(stripShellWrapper(command))
	switch {
	case strings.HasPrefix(command, "rg ") || command == "rg" ||
		strings.HasPrefix(command, "grep ") || command == "grep" ||
		strings.HasPrefix(command, "find ") || command == "find":
		return "search"
	case strings.HasPrefix(command, "sed ") || command == "sed" ||
		strings.HasPrefix(command, "cat ") || command == "cat" ||
		strings.HasPrefix(command, "nl ") || command == "nl" ||
		strings.HasPrefix(command, "head ") || command == "head" ||
		strings.HasPrefix(command, "tail ") || command == "tail" ||
		strings.HasPrefix(command, "ls ") || command == "ls":
		return "read"
	case strings.HasPrefix(command, "./test.sh") ||
		strings.HasPrefix(command, "go test") ||
		strings.HasPrefix(command, "npm test") ||
		strings.HasPrefix(command, "pytest"):
		return "test"
	case strings.HasPrefix(command, "./build.sh") ||
		strings.HasPrefix(command, "go build") ||
		strings.HasPrefix(command, "npm run build"):
		return "build"
	case strings.HasPrefix(command, "git ") || command == "git":
		return "git"
	default:
		return "other"
	}
}

var evidenceLocationPattern = regexp.MustCompile(`([A-Za-z0-9_./-]+\.[A-Za-z0-9_]+):([0-9]+)(?:-([0-9]+))?`)

func evidenceFieldsFromText(text string) map[string]any {
	matches := evidenceLocationPattern.FindAllStringSubmatch(text, 5)
	if len(matches) == 0 {
		return nil
	}
	locations := make([]string, 0, len(matches))
	firstFile := ""
	firstLine := 0
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		file := match[1]
		line, err := strconv.Atoi(match[2])
		if err != nil || line <= 0 {
			continue
		}
		location := file + ":" + match[2]
		if len(match) > 3 && match[3] != "" {
			location += "-" + match[3]
		}
		locations = append(locations, location)
		if firstFile == "" {
			firstFile = file
			firstLine = line
		}
	}
	return compactMap(map[string]any{
		"evidence_file":      firstFile,
		"evidence_line":      intIfPositive(firstLine),
		"evidence_locations": strings.Join(locations, ","),
	})
}

func fileChangeMessage(fields map[string]any) string {
	if fields == nil {
		return "File change completed"
	}
	location := stringField(fields, "file_locations")
	if location == "" {
		location = stringField(fields, "files")
	}
	summary := stringField(fields, "summary")
	if location != "" && summary != "" {
		return fmt.Sprintf("Changed %s (%s)", location, summary)
	}
	if location != "" {
		return "Changed " + location
	}
	return "File change completed"
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
