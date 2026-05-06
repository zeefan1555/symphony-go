package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	runtimeconfig "symphony-go/internal/runtime/config"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/workspace"
)

const (
	initializeID  = 1
	threadStartID = 2
	turnStartID   = 3
)

type Runner struct {
	Config       runtimeconfig.CodexConfig
	dynamicTools *DynamicToolExecutor
}

type LinearGraphQLExecutor func(context.Context, string, map[string]any) (map[string]any, error)

type linearGraphQLExecutor struct {
	exec LinearGraphQLExecutor
}

func (e linearGraphQLExecutor) GraphQLRaw(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	if e.exec == nil {
		return nil, fmt.Errorf("Linear GraphQL executor is not configured")
	}
	return e.exec(ctx, query, variables)
}

type Event struct {
	Name    string
	Payload map[string]any
	Raw     string
}

type Result struct {
	SessionID string
	ThreadID  string
	TurnID    string
	PID       int
}

type SessionRequest struct {
	WorkspacePath string
	Issue         issuemodel.Issue
	Prompts       []TurnPrompt
	AfterTurn     func(context.Context, Result, int) (TurnPrompt, bool, error)
}

type TurnPrompt struct {
	Text         string
	Continuation bool
	Attempt      *int
	Issue        *issuemodel.Issue
}

type SessionResult struct {
	SessionID string
	ThreadID  string
	Turns     []Result
	PID       int
}

type Option func(*Runner)

func WithDynamicToolExecutor(executor *DynamicToolExecutor) Option {
	return func(r *Runner) {
		r.dynamicTools = executor
	}
}

func New(cfg runtimeconfig.CodexConfig, opts ...Option) *Runner {
	r := &Runner{Config: cfg}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func NewWithLinearGraphQL(cfg runtimeconfig.CodexConfig, exec LinearGraphQLExecutor) *Runner {
	return New(cfg, WithDynamicToolExecutor(NewDynamicToolExecutor(linearGraphQLExecutor{exec: exec})))
}

func (r *Runner) DynamicToolSpecs() []any {
	if r == nil || r.dynamicTools == nil {
		return []any{}
	}
	return r.dynamicTools.ToolSpecs()
}

func (r *Runner) Run(ctx context.Context, workspacePath string, prompt string, issue issuemodel.Issue, onEvent func(Event)) (Result, error) {
	result, err := r.RunSession(ctx, SessionRequest{
		WorkspacePath: workspacePath,
		Issue:         issue,
		Prompts:       []TurnPrompt{{Text: prompt}},
	}, onEvent)
	if err != nil {
		if len(result.Turns) > 0 {
			return result.Turns[len(result.Turns)-1], err
		}
		return Result{}, err
	}
	if len(result.Turns) == 0 {
		return Result{}, fmt.Errorf("codex session completed without turns")
	}
	return result.Turns[len(result.Turns)-1], nil
}

func (r *Runner) RunSession(ctx context.Context, request SessionRequest, onEvent func(Event)) (SessionResult, error) {
	if len(request.Prompts) == 0 {
		return SessionResult{}, fmt.Errorf("codex session requires at least one prompt")
	}
	session, err := r.startSession(ctx, request.WorkspacePath)
	if err != nil {
		return SessionResult{}, err
	}
	defer session.Close()

	sessionResult := SessionResult{
		ThreadID: session.threadID,
		PID:      session.pid(),
	}
	if onEvent != nil {
		onEvent(Event{Name: "session_started", Payload: map[string]any{"session_id": sessionResult.ThreadID, "thread_id": sessionResult.ThreadID, "pid": sessionResult.PID}})
	}
	for turnIndex := 0; turnIndex < len(request.Prompts); turnIndex++ {
		prompt := request.Prompts[turnIndex]
		turnIssue := request.Issue
		if prompt.Issue != nil {
			turnIssue = *prompt.Issue
		}
		turnID, err := session.startTurn(turnStartID+turnIndex, request.WorkspacePath, prompt.Text, turnIssue, r.Config.ApprovalPolicy, r.turnSandboxPolicy(request.WorkspacePath, turnIssue))
		if err != nil {
			return sessionResult, err
		}
		result := Result{
			SessionID: session.threadID + "-" + turnID,
			ThreadID:  session.threadID,
			TurnID:    turnID,
			PID:       session.pid(),
		}
		sessionResult.SessionID = result.SessionID
		sessionResult.ThreadID = result.ThreadID
		sessionResult.PID = result.PID
		sessionResult.Turns = append(sessionResult.Turns, result)
		if onEvent != nil {
			onEvent(Event{Name: "turn_started", Payload: map[string]any{
				"session_id":   result.SessionID,
				"thread_id":    result.ThreadID,
				"turn_id":      result.TurnID,
				"pid":          result.PID,
				"turn_count":   turnIndex + 1,
				"continuation": prompt.Continuation,
			}})
		}
		if err := session.awaitTurn(ctx, time.Duration(r.Config.TurnTimeoutMS)*time.Millisecond, onEvent); err != nil {
			return sessionResult, err
		}
		if onEvent != nil {
			onEvent(Event{Name: "turn_completed", Payload: map[string]any{
				"session_id":   result.SessionID,
				"thread_id":    result.ThreadID,
				"turn_id":      result.TurnID,
				"pid":          result.PID,
				"turn_count":   turnIndex + 1,
				"continuation": prompt.Continuation,
			}})
		}
		if request.AfterTurn == nil {
			continue
		}
		nextPrompt, ok, err := request.AfterTurn(ctx, result, turnIndex+1)
		if err != nil {
			return sessionResult, err
		}
		if !ok {
			return sessionResult, nil
		}
		request.Prompts = append(request.Prompts, nextPrompt)
	}
	return sessionResult, nil
}

func (r *Runner) startSession(ctx context.Context, workspacePath string) (*session, error) {
	cmd := exec.CommandContext(ctx, "bash", "-lc", r.Config.Command)
	cmd.Dir = workspacePath
	cmd.Env = workspace.UTF8Env(os.Environ())
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go io.Copy(io.Discard, stderr)

	s := &session{
		cmd:          cmd,
		stdin:        stdin,
		scanner:      bufio.NewScanner(stdout),
		readTimeout:  time.Duration(r.Config.ReadTimeoutMS) * time.Millisecond,
		lines:        make(chan lineResult),
		dynamicTools: r.dynamicTools,
	}
	s.scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	go s.readLines()
	if err := s.initialize(); err != nil {
		s.Close()
		return nil, err
	}
	threadID, err := s.startThread(workspacePath, r.Config.ApprovalPolicy, r.Config.ThreadSandbox)
	if err != nil {
		s.Close()
		return nil, err
	}
	s.threadID = threadID
	return s, nil
}

func (r *Runner) turnSandboxPolicy(workspacePath string, issue issuemodel.Issue) map[string]any {
	policy := map[string]any{}
	for key, value := range r.Config.TurnSandboxPolicy {
		policy[key] = value
	}
	if len(policy) == 0 {
		policy = map[string]any{
			"type":          "workspaceWrite",
			"writableRoots": []any{workspacePath},
			"networkAccess": true,
		}
	}
	if policy["type"] == "workspaceWrite" {
		roots := toStringSlice(policy["writableRoots"])
		roots = appendUnique(roots, workspacePath)
		for _, root := range gitMetadataRoots(workspacePath) {
			roots = appendUnique(roots, root)
		}
		values := make([]any, 0, len(roots))
		for _, root := range roots {
			values = append(values, root)
		}
		policy["writableRoots"] = values
	}
	return policy
}

type session struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	scanner      *bufio.Scanner
	readTimeout  time.Duration
	threadID     string
	lines        chan lineResult
	dynamicTools *DynamicToolExecutor
	mu           sync.Mutex
}

type lineResult struct {
	line string
	err  error
}

func (s *session) Close() {
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	}
}

func (s *session) pid() int {
	if s.cmd == nil || s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

func (s *session) initialize() error {
	if err := s.send(map[string]any{
		"method": "initialize",
		"id":     initializeID,
		"params": map[string]any{
			"capabilities": map[string]any{"experimentalApi": true},
			"clientInfo": map[string]any{
				"name":    "symphony-go-orchestrator",
				"title":   "Symphony Go Orchestrator",
				"version": "0.1.0",
			},
		},
	}); err != nil {
		return err
	}
	if _, err := s.awaitResponse(initializeID); err != nil {
		return err
	}
	return s.send(map[string]any{"method": "initialized", "params": map[string]any{}})
}

func (s *session) startThread(cwd string, approvalPolicy any, sandbox string) (string, error) {
	if err := s.send(map[string]any{
		"method": "thread/start",
		"id":     threadStartID,
		"params": map[string]any{
			"approvalPolicy": approvalPolicy,
			"sandbox":        sandbox,
			"cwd":            cwd,
			"dynamicTools":   s.dynamicToolSpecs(),
		},
	}); err != nil {
		return "", err
	}
	result, err := s.awaitResponse(threadStartID)
	if err != nil {
		return "", err
	}
	thread, _ := result["thread"].(map[string]any)
	threadID, _ := thread["id"].(string)
	if threadID == "" {
		return "", fmt.Errorf("invalid thread/start response: %v", result)
	}
	return threadID, nil
}

func (s *session) dynamicToolSpecs() []any {
	if s.dynamicTools == nil {
		return []any{}
	}
	return s.dynamicTools.ToolSpecs()
}

func (s *session) startTurn(id int, cwd string, prompt string, issue issuemodel.Issue, approvalPolicy any, sandboxPolicy map[string]any) (string, error) {
	if err := s.send(map[string]any{
		"method": "turn/start",
		"id":     id,
		"params": map[string]any{
			"threadId":       s.threadID,
			"input":          []any{map[string]any{"type": "text", "text": prompt}},
			"cwd":            cwd,
			"title":          issue.Identifier + ": " + issue.Title,
			"approvalPolicy": approvalPolicy,
			"sandboxPolicy":  sandboxPolicy,
		},
	}); err != nil {
		return "", err
	}
	result, err := s.awaitResponse(id)
	if err != nil {
		return "", err
	}
	turn, _ := result["turn"].(map[string]any)
	turnID, _ := turn["id"].(string)
	if turnID == "" {
		return "", fmt.Errorf("invalid turn/start response: %v", result)
	}
	return turnID, nil
}

func (s *session) awaitTurn(ctx context.Context, timeout time.Duration, onEvent func(Event)) error {
	if timeout <= 0 {
		timeout = time.Hour
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("turn timeout after %s", timeout)
		case result := <-s.lines:
			if result.err != nil {
				return result.err
			}
			line := result.line
			var payload map[string]any
			if err := json.Unmarshal([]byte(line), &payload); err != nil {
				continue
			}
			method, _ := payload["method"].(string)
			if onEvent != nil {
				onEvent(Event{Name: method, Payload: payload, Raw: line})
			}
			switch method {
			case "turn/completed":
				return nil
			case "turn/failed", "turn/cancelled":
				return fmt.Errorf("%s: %v", method, payload["params"])
			case "turn/input_required", "turn/approval_required", "item/tool/requestUserInput", "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
				return fmt.Errorf("%s: unattended runs fail instead of waiting for user input or approval: %v", method, payload["params"])
			case "item/tool/call":
				if err := s.handleDynamicToolCall(ctx, payload); err != nil {
					return err
				}
			case "mcpServer/elicitation/request":
				return fmt.Errorf("codex requested interactive MCP approval; unattended runs must not use MCP write tools: %v", payload["params"])
			}
		}
	}
}

func (s *session) handleDynamicToolCall(ctx context.Context, payload map[string]any) error {
	requestID, ok := payload["id"]
	if !ok {
		return fmt.Errorf("dynamic tool call missing id: %v", payload)
	}
	params, _ := payload["params"].(map[string]any)
	executor := s.dynamicTools
	if executor == nil {
		executor = NewDynamicToolExecutor(nil)
	}
	result := executor.Execute(ctx, toolCallName(params), toolCallArguments(params))
	return s.send(map[string]any{
		"id":     requestID,
		"result": result,
	})
}

func toolCallName(params map[string]any) string {
	if params == nil {
		return ""
	}
	for _, key := range []string{"tool", "name", "toolName"} {
		if raw, ok := params[key].(string); ok {
			if name := strings.TrimSpace(raw); name != "" {
				return name
			}
		}
	}
	return ""
}

func toolCallArguments(params map[string]any) any {
	if params == nil {
		return map[string]any{}
	}
	if arguments, ok := params["arguments"]; ok {
		return arguments
	}
	return map[string]any{}
}

func (s *session) awaitResponse(id int) (map[string]any, error) {
	timeout := time.NewTimer(s.readTimeout)
	defer timeout.Stop()
	for {
		select {
		case <-timeout.C:
			return nil, fmt.Errorf("response timeout waiting for id=%d", id)
		case result := <-s.lines:
			if result.err != nil {
				return nil, result.err
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(result.line), &payload); err != nil {
				continue
			}
			responseID, ok := numericID(payload["id"])
			if !ok || responseID != id {
				continue
			}
			if rawErr, ok := payload["error"]; ok {
				return nil, fmt.Errorf("response error id=%d: %v", id, rawErr)
			}
			responseResult, _ := payload["result"].(map[string]any)
			if responseResult == nil {
				return nil, fmt.Errorf("response id=%d missing result", id)
			}
			return responseResult, nil
		}
	}
}

func (s *session) readLines() {
	for s.scanner.Scan() {
		s.lines <- lineResult{line: s.scanner.Text()}
	}
	if err := s.scanner.Err(); err != nil {
		s.lines <- lineResult{err: err}
		return
	}
	s.lines <- lineResult{err: io.EOF}
}

func (s *session) send(message map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = s.stdin.Write(raw)
	return err
}

func numericID(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

func toStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	roots := make([]string, 0, len(items))
	for _, item := range items {
		if root, ok := item.(string); ok && root != "" {
			roots = append(roots, root)
		}
	}
	return roots
}

func appendUnique(items []string, value string) []string {
	if value == "" {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func gitMetadataRoots(workspacePath string) []string {
	var roots []string
	for _, flag := range []string{"--git-dir", "--git-common-dir"} {
		roots = appendUnique(roots, gitRevParsePath(workspacePath, flag))
	}
	return roots
}

func gitRevParsePath(workspacePath, flag string) string {
	cmd := exec.Command("git", "-C", workspacePath, "rev-parse", flag)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	root := filepath.Clean(strings.TrimSpace(string(output)))
	if !filepath.IsAbs(root) {
		root = filepath.Join(workspacePath, root)
	}
	return root
}
