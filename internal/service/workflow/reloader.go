package workflow

import (
	"fmt"
	"os"
	"sync"
	"time"

	runtimeconfig "symphony-go/internal/runtime/config"
)

const stableLoadAttempts = 3

type Reloader struct {
	path           string
	mu             sync.RWMutex
	current        *runtimeconfig.Workflow
	modTime        time.Time
	size           int64
	pending        *runtimeconfig.Workflow
	pendingModTime time.Time
	pendingSize    int64
}

func NewReloader(path string) (*Reloader, error) {
	return newReloader(path, os.Stat, Load)
}

func newReloader(path string, stat func(string) (os.FileInfo, error), load func(string) (*runtimeconfig.Workflow, error)) (*Reloader, error) {
	loaded, info, err := loadStableWorkflow(path, stat, load)
	if err != nil {
		return nil, err
	}
	return &Reloader{
		path:    path,
		current: loaded,
		modTime: info.ModTime(),
		size:    info.Size(),
	}, nil
}

func loadStableWorkflow(path string, stat func(string) (os.FileInfo, error), load func(string) (*runtimeconfig.Workflow, error)) (*runtimeconfig.Workflow, os.FileInfo, error) {
	for attempt := 0; attempt < stableLoadAttempts; attempt++ {
		before, err := stat(path)
		if err != nil {
			return nil, nil, err
		}
		loaded, err := load(path)
		if err != nil {
			return nil, nil, err
		}
		after, err := stat(path)
		if err != nil {
			return nil, nil, err
		}
		if sameFileInfo(before, after) {
			return loaded, after, nil
		}
	}
	return nil, nil, fmt.Errorf("workflow changed while loading")
}

func sameFileInfo(left, right os.FileInfo) bool {
	return left.ModTime().Equal(right.ModTime()) && left.Size() == right.Size()
}

func (r *Reloader) Current() *runtimeconfig.Workflow {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneWorkflow(r.current)
}

func (r *Reloader) ReloadIfChanged() (*runtimeconfig.Workflow, bool, error) {
	info, err := os.Stat(r.path)
	if err != nil {
		return nil, false, err
	}
	r.mu.RLock()
	same := info.ModTime().Equal(r.modTime) && info.Size() == r.size
	r.mu.RUnlock()
	if same {
		return nil, false, nil
	}
	loaded, err := Load(r.path)
	if err != nil {
		return nil, false, err
	}
	r.mu.Lock()
	r.pending = loaded
	r.pendingModTime = info.ModTime()
	r.pendingSize = info.Size()
	r.mu.Unlock()
	return cloneWorkflow(loaded), true, nil
}

func (r *Reloader) CommitCandidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		return
	}
	r.current = r.pending
	r.modTime = r.pendingModTime
	r.size = r.pendingSize
	r.pending = nil
}

func cloneWorkflow(input *runtimeconfig.Workflow) *runtimeconfig.Workflow {
	if input == nil {
		return nil
	}
	copy := *input
	copy.Config.Tracker.ActiveStates = append([]string(nil), input.Config.Tracker.ActiveStates...)
	copy.Config.Tracker.TerminalStates = append([]string(nil), input.Config.Tracker.TerminalStates...)
	copy.Config.Worker.SSHHosts = append([]string(nil), input.Config.Worker.SSHHosts...)
	if input.Config.Agent.MaxConcurrentAgentsByState != nil {
		copy.Config.Agent.MaxConcurrentAgentsByState = map[string]int{}
		for key, value := range input.Config.Agent.MaxConcurrentAgentsByState {
			copy.Config.Agent.MaxConcurrentAgentsByState[key] = value
		}
	}
	copy.Config.Codex.TurnSandboxPolicy = cloneStringAnyMap(input.Config.Codex.TurnSandboxPolicy)
	return &copy
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneAny(value)
	}
	return output
}

func cloneAny(input any) any {
	switch value := input.(type) {
	case map[string]any:
		return cloneStringAnyMap(value)
	case []any:
		output := make([]any, len(value))
		for i, item := range value {
			output[i] = cloneAny(item)
		}
		return output
	default:
		return input
	}
}
