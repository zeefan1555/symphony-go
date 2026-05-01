package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zeefan1555/symphony-go/internal/types"
)

var unsafeIdentifierChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

type Manager struct {
	Root         string
	Hooks        types.HooksConfig
	hookObserver HookObserver
}

func New(root string, hooks types.HooksConfig) *Manager {
	return &Manager{Root: root, Hooks: hooks}
}

type HookEvent struct {
	Name            string
	Stage           string
	Source          string
	Script          string
	CWD             string
	Duration        time.Duration
	Output          string
	Err             error
	IssueID         string
	IssueIdentifier string
}

type HookObserver func(HookEvent)

type hookIssueContextKey struct{}
type hookSourceContextKey struct{}

type hookIssue struct {
	ID         string
	Identifier string
}

func WithHookIssue(ctx context.Context, issue types.Issue) context.Context {
	if issue.ID == "" && issue.Identifier == "" {
		return ctx
	}
	return context.WithValue(ctx, hookIssueContextKey{}, hookIssue{
		ID:         issue.ID,
		Identifier: issue.Identifier,
	})
}

func WithHookSource(ctx context.Context, source string) context.Context {
	if source == "" {
		return ctx
	}
	return context.WithValue(ctx, hookSourceContextKey{}, source)
}

func (m *Manager) SetHookObserver(observer HookObserver) {
	m.hookObserver = observer
}

func (m *Manager) RootAbs() (string, error) {
	root, err := filepath.Abs(expandHome(m.Root))
	if err != nil {
		return "", err
	}
	return filepath.Clean(root), nil
}

func (m *Manager) PathForIssue(issue types.Issue) (string, error) {
	if issue.Identifier == "" {
		return "", fmt.Errorf("issue identifier is required")
	}
	root, err := m.RootAbs()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, SafeIdentifier(issue.Identifier)), nil
}

func (m *Manager) ValidateWorkspacePath(path string) error {
	root, abs, err := m.validateWorkspacePathLexical(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	realRoot, err := evalIfExists(root)
	if err != nil {
		return err
	}
	existing, err := nearestExistingPath(abs)
	if err != nil {
		return err
	}
	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return err
	}
	if !pathWithin(realRoot, realExisting) {
		return fmt.Errorf("workspace path %q escapes workspace root %q", abs, root)
	}
	return nil
}

func (m *Manager) validateWorkspacePathLexical(path string) (string, string, error) {
	root, err := m.RootAbs()
	if err != nil {
		return "", "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	abs = filepath.Clean(abs)
	root = filepath.Clean(root)
	if !pathWithin(root, abs) {
		return "", "", fmt.Errorf("workspace path %q escapes workspace root %q", abs, root)
	}
	return root, abs, nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func evalIfExists(path string) (string, error) {
	evaluated, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(evaluated), nil
	}
	if os.IsNotExist(err) {
		return filepath.Clean(path), nil
	}
	return "", err
}

func nearestExistingPath(path string) (string, error) {
	for {
		if _, err := os.Lstat(path); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", fmt.Errorf("no existing parent for workspace path %q", path)
		}
		path = parent
	}
}

func (m *Manager) BeforeRun(ctx context.Context, path string) error {
	if err := m.ValidateWorkspacePath(path); err != nil {
		return err
	}
	if m.Hooks.BeforeRun == "" {
		return nil
	}
	return m.runHook(ctx, "before_run", m.Hooks.BeforeRun, path)
}

func (m *Manager) AfterRun(ctx context.Context, path string) error {
	if err := m.ValidateWorkspacePath(path); err != nil {
		return err
	}
	if m.Hooks.AfterRun == "" {
		return nil
	}
	return m.runHook(ctx, "after_run", m.Hooks.AfterRun, path)
}

func (m *Manager) Ensure(ctx context.Context, issue types.Issue) (string, bool, error) {
	root, err := m.RootAbs()
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", false, err
	}
	path, err := m.PathForIssue(issue)
	if err != nil {
		return "", false, err
	}
	if _, _, err := m.validateWorkspacePathLexical(path); err != nil {
		return "", false, err
	}
	created := false
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(path); err != nil {
			return "", false, err
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", false, err
		}
		created = true
	} else if err == nil && info.IsDir() {
		if err := m.ValidateWorkspacePath(path); err != nil {
			return "", false, err
		}
		created = false
	} else {
		if err != nil && !os.IsNotExist(err) {
			return "", false, err
		}
		if err := m.ValidateWorkspacePath(path); err != nil {
			return "", false, err
		}
		if err := os.RemoveAll(path); err != nil {
			return "", false, err
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", false, err
		}
		created = true
	}
	if created && m.Hooks.AfterCreate != "" {
		if err := m.runHook(ctx, "after_create", m.Hooks.AfterCreate, path); err != nil {
			return "", false, err
		}
	}
	return path, created, nil
}

func (m *Manager) Remove(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}
	if _, _, err := m.validateWorkspacePathLexical(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if err := m.ValidateWorkspacePath(filepath.Dir(path)); err != nil {
			return err
		}
		return os.Remove(path)
	}
	if err := m.ValidateWorkspacePath(path); err != nil {
		return err
	}
	if m.Hooks.BeforeRemove != "" {
		if info.IsDir() {
			_ = m.runHook(ctx, "before_remove", m.Hooks.BeforeRemove, path)
		}
	}
	return os.RemoveAll(path)
}

func SafeIdentifier(identifier string) string {
	if identifier == "" {
		return "issue"
	}
	safe := unsafeIdentifierChars.ReplaceAllString(identifier, "_")
	if strings.Trim(safe, ".") == "" {
		return "issue"
	}
	return safe
}

func (m *Manager) runHook(ctx context.Context, name, script, cwd string) error {
	timeout := time.Duration(m.Hooks.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(hookCtx, "sh", "-lc", script)
	cmd.Dir = cwd
	cmd.Env = utf8Env(os.Environ())
	m.emitHook(ctx, HookEvent{
		Name:   name,
		Stage:  "started",
		Script: script,
		CWD:    cwd,
	})
	startedAt := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(startedAt)
	if hookCtx.Err() == context.DeadlineExceeded {
		hookErr := fmt.Errorf("%s hook timed out after %s", name, timeout)
		m.emitHook(ctx, HookEvent{
			Name:     name,
			Stage:    "timed_out",
			Script:   script,
			CWD:      cwd,
			Duration: duration,
			Output:   string(output),
			Err:      hookErr,
		})
		return hookErr
	}
	if err != nil {
		hookErr := fmt.Errorf("%s hook failed: %w: %s", name, err, string(output))
		m.emitHook(ctx, HookEvent{
			Name:     name,
			Stage:    "failed",
			Script:   script,
			CWD:      cwd,
			Duration: duration,
			Output:   string(output),
			Err:      hookErr,
		})
		return hookErr
	}
	m.emitHook(ctx, HookEvent{
		Name:     name,
		Stage:    "completed",
		Script:   script,
		CWD:      cwd,
		Duration: duration,
		Output:   string(output),
	})
	return nil
}

func (m *Manager) emitHook(ctx context.Context, event HookEvent) {
	if m.hookObserver == nil {
		return
	}
	if issue, ok := ctx.Value(hookIssueContextKey{}).(hookIssue); ok {
		event.IssueID = issue.ID
		event.IssueIdentifier = issue.Identifier
	}
	if source, ok := ctx.Value(hookSourceContextKey{}).(string); ok {
		event.Source = source
	}
	m.hookObserver(event)
}

func utf8Env(env []string) []string {
	return appendWithDefault(appendWithDefault(env, "LANG", "en_US.UTF-8"), "LC_ALL", "en_US.UTF-8")
}

func appendWithDefault(env []string, key, value string) []string {
	prefix := key + "="
	for _, item := range env {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			return env
		}
	}
	return append(env, prefix+value)
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if len(path) > 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func UTF8Env(env []string) []string {
	return utf8Env(env)
}
