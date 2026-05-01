package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zeefan1555/symphony-go/internal/codex"
	"github.com/zeefan1555/symphony-go/internal/logging"
	"github.com/zeefan1555/symphony-go/internal/observability"
	"github.com/zeefan1555/symphony-go/internal/types"
	"github.com/zeefan1555/symphony-go/internal/workflow"
	"github.com/zeefan1555/symphony-go/internal/workspace"
)

type Tracker interface {
	FetchActiveIssues(context.Context, []string) ([]types.Issue, error)
	FetchIssuesByStates(context.Context, []string) ([]types.Issue, error)
	FetchIssue(context.Context, string) (types.Issue, error)
	FetchIssueStatesByIDs(context.Context, []string) ([]types.Issue, error)
	UpdateIssueState(context.Context, string, string) error
	UpsertWorkpad(context.Context, string, string) error
}

type AgentRunner interface {
	RunSession(context.Context, codex.SessionRequest, func(codex.Event)) (codex.SessionResult, error)
}

const continuationPromptText = "Continue working on the same issue. Re-check the current workspace state, finish any remaining acceptance criteria from the issue, run the smallest relevant verification, and report concrete progress or blockers. Do not repeat completed work."

var errNoRetryNeeded = errors.New("no retry needed")

type WorkflowReloader interface {
	Current() *types.Workflow
	ReloadIfChanged() (*types.Workflow, bool, error)
	CommitCandidate()
}

type Options struct {
	Workflow         *types.Workflow
	Reloader         WorkflowReloader
	Tracker          Tracker
	TrackerFactory   func(types.TrackerConfig) (Tracker, error)
	Workspace        *workspace.Manager
	WorkspaceFactory func(types.WorkspaceConfig, types.HooksConfig) *workspace.Manager
	Runner           AgentRunner
	RunnerFactory    func(types.CodexConfig) AgentRunner
	NewTimer         func(time.Duration, func()) *time.Timer
	Logger           *logging.Logger
	Once             bool
	IssueFilter      string
	RepoRoot         string
	MergeTarget      string
}

type Orchestrator struct {
	opts                   Options
	mu                     sync.RWMutex
	snapshot               observability.Snapshot
	claimed                map[string]bool
	runningCancel          map[string]context.CancelFunc
	pendingTerminalCleanup map[string]terminalCleanup
	serviceCtx             context.Context
	retryTimers            map[string]*time.Timer
	retryAttempts          map[string]int
	newTimer               func(time.Duration, func()) *time.Timer
	pollNow                chan struct{}
}

type runtimeSnapshot struct {
	workflow    *types.Workflow
	tracker     Tracker
	workspace   *workspace.Manager
	runner      AgentRunner
	repoRoot    string
	mergeTarget string
}

type terminalCleanup struct {
	issue types.Issue
	entry observability.RunningEntry
}

func New(opts Options) *Orchestrator {
	snapshot := observability.NewSnapshot()
	snapshot.Polling.IntervalMS = int(pollingInterval(opts) / time.Millisecond)
	newTimer := opts.NewTimer
	if newTimer == nil {
		newTimer = time.AfterFunc
	}
	o := &Orchestrator{
		opts:                   opts,
		snapshot:               snapshot,
		claimed:                map[string]bool{},
		runningCancel:          map[string]context.CancelFunc{},
		pendingTerminalCleanup: map[string]terminalCleanup{},
		retryTimers:            map[string]*time.Timer{},
		retryAttempts:          map[string]int{},
		newTimer:               newTimer,
		pollNow:                make(chan struct{}, 1),
	}
	o.configureWorkspaceObserver(o.opts.Workspace)
	return o
}

func (o *Orchestrator) Snapshot() observability.Snapshot {
	o.mu.RLock()
	defer o.mu.RUnlock()

	now := time.Now()
	snapshot := o.snapshot
	snapshot.GeneratedAt = now
	snapshot.Running = cloneRunning(snapshot.Running)
	snapshot.Retrying = cloneRetrying(snapshot.Retrying)
	snapshot.RateLimits = cloneJSONValue(snapshot.RateLimits)
	snapshot.Counts.Running = len(snapshot.Running)
	snapshot.Counts.Retrying = len(snapshot.Retrying)
	snapshot.CodexTotals.SecondsRunning = snapshot.TotalRuntimeSeconds(now)
	snapshot.Polling.NextPollInMS = millisUntil(now, snapshot.Polling.NextPollAt)
	return snapshot
}

func (o *Orchestrator) Run(ctx context.Context) error {
	o.setServiceContext(ctx)
	defer o.stopRetryTimers()

	interval := pollingInterval(o.currentOptions())
	for {
		o.markPolling(true, time.Time{})
		dispatched, err := o.pollDispatched(ctx)
		if err != nil {
			o.log("", "poll_error", err.Error(), nil)
		}
		interval = pollingInterval(o.currentOptions())
		o.markPolling(false, time.Now().Add(interval))
		if o.currentOptions().Once {
			return waitForDispatched(ctx, dispatched)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-o.pollNow:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (o *Orchestrator) poll(ctx context.Context) error {
	_, err := o.pollDispatched(ctx)
	return err
}

func (o *Orchestrator) pollDispatched(ctx context.Context) ([]<-chan struct{}, error) {
	reloadFailed := o.refreshWorkflow()
	rt := o.currentRuntime()
	if err := o.reconcileRunning(ctx); err != nil {
		o.log("", "reconcile_error", err.Error(), nil)
	}
	issues, err := rt.tracker.FetchActiveIssues(ctx, rt.workflow.Config.Tracker.ActiveStates)
	if err != nil {
		o.setLastError(err.Error())
		return nil, err
	}
	if !reloadFailed {
		o.clearLastError()
	}
	var dispatched []<-chan struct{}
	sortCandidates(issues)
	for _, issue := range issues {
		opts := o.currentOptions()
		if opts.IssueFilter != "" && issue.Identifier != opts.IssueFilter && issue.ID != opts.IssueFilter {
			continue
		}
		ok, reason := candidateEligible(issue, o.eligibilityState())
		if !ok {
			if reason == "no_available_orchestrator_slots" && o.globalSlotsAvailable() <= 0 {
				break
			}
			if reason == "waiting_for_review" {
				o.logIssue(issue, "waiting_for_review", "issue is waiting for human review", nil)
				continue
			}
			o.logIssue(issue, "dispatch_skipped", reason, nil)
			continue
		}
		fmt.Printf("issue=%s state=%s title=%s\n", issue.Identifier, issue.State, issue.Title)
		done, ok := o.dispatchIssueDone(ctx, issue, 0)
		if !ok {
			o.logIssue(issue, "dispatch_skipped", "claimed", nil)
			continue
		}
		dispatched = append(dispatched, done)
	}
	return dispatched, nil
}

func (o *Orchestrator) reconcileRunning(ctx context.Context) error {
	o.detectStalledRunning()

	rt := o.currentRuntime()
	entries := o.runningEntries()
	if len(entries) == 0 {
		return nil
	}
	ids := make([]string, 0, len(entries))
	byID := make(map[string]observability.RunningEntry, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.IssueID)
		byID[entry.IssueID] = entry
	}
	issues, err := rt.tracker.FetchIssueStatesByIDs(ctx, ids)
	if err != nil {
		o.log("", "reconcile_refresh_failed", err.Error(), map[string]any{"issue_ids": ids})
		return nil
	}
	for _, issue := range issues {
		entry, ok := byID[issue.ID]
		if !ok {
			continue
		}
		if isTerminal(issue.State, rt.workflow.Config.Tracker.TerminalStates) {
			o.markPendingTerminalCleanup(issue, entry)
			o.cancelRunning(issue.ID)
			continue
		}
		if isActive(issue.State, rt.workflow.Config.Tracker.ActiveStates) {
			o.updateRunningState(issue.ID, issue.State)
			continue
		}
		o.cancelRunning(issue.ID)
	}
	return nil
}

func (o *Orchestrator) StartupCleanup(ctx context.Context) {
	rt := o.currentRuntime()
	terminalStates := rt.workflow.Config.Tracker.TerminalStates
	if len(terminalStates) == 0 {
		return
	}
	issues, err := rt.tracker.FetchIssuesByStates(ctx, terminalStates)
	if err != nil {
		o.log("", "startup_cleanup_fetch_failed", err.Error(), map[string]any{"states": terminalStates})
		return
	}
	cleanupCtx := workspace.WithHookSource(ctx, "startup_cleanup")
	for _, issue := range issues {
		o.cleanupWorkspace(cleanupCtx, rt.workspace, issue, observability.RunningEntry{})
	}
}

func (o *Orchestrator) detectStalledRunning() {
	opts := o.currentOptions()
	timeoutMS := opts.Workflow.Config.Codex.StallTimeoutMS
	if timeoutMS <= 0 {
		return
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond
	now := time.Now()
	for _, entry := range o.runningEntries() {
		if o.retryQueued(entry.IssueID) {
			continue
		}
		last := entry.LastEventAt
		if last.IsZero() {
			last = entry.StartedAt
		}
		if last.IsZero() || now.Sub(last) <= timeout {
			continue
		}
		issue := types.Issue{
			ID:         entry.IssueID,
			Identifier: entry.IssueIdentifier,
			State:      entry.State,
		}
		err := fmt.Errorf("stalled after %s", timeout)
		o.scheduleRetry(issue, 1, retryFailure, err)
		o.cancelRunning(entry.IssueID)
		o.logIssue(types.Issue{ID: entry.IssueID, Identifier: entry.IssueIdentifier}, "worker_stalled", err.Error(), nil)
	}
}

func (o *Orchestrator) runningEntries() []observability.RunningEntry {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return cloneRunning(o.snapshot.Running)
}

func (o *Orchestrator) retryQueued(issueID string) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	_, ok := o.retryAttempts[issueID]
	return ok
}

func (o *Orchestrator) cancelRunning(issueID string) {
	o.mu.RLock()
	cancel := o.runningCancel[issueID]
	o.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
}

func (o *Orchestrator) updateRunningState(issueID, state string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i := range o.snapshot.Running {
		if o.snapshot.Running[i].IssueID == issueID {
			o.snapshot.Running[i].State = state
			return
		}
	}
}

func (o *Orchestrator) markPendingTerminalCleanup(issue types.Issue, entry observability.RunningEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pendingTerminalCleanup[issue.ID] = terminalCleanup{issue: issue, entry: entry}
}

func (o *Orchestrator) popPendingTerminalCleanup(issueID string) (terminalCleanup, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	cleanup, ok := o.pendingTerminalCleanup[issueID]
	if ok {
		delete(o.pendingTerminalCleanup, issueID)
	}
	return cleanup, ok
}

func (o *Orchestrator) cleanupWorkspace(ctx context.Context, manager *workspace.Manager, issue types.Issue, entry observability.RunningEntry) {
	path := entry.WorkspacePath
	if path == "" {
		var err error
		path, err = manager.PathForIssue(issue)
		if err != nil {
			o.logIssue(issue, "workspace_cleanup_failed", err.Error(), nil)
			return
		}
	}
	if err := manager.Remove(workspace.WithHookIssue(ctx, issue), path); err != nil {
		o.logIssue(issue, "workspace_cleanup_failed", err.Error(), map[string]any{"workspace_path": path})
		return
	}
	o.logIssue(issue, "workspace_cleaned", "workspace removed", map[string]any{"workspace_path": path})
}

func waitForDispatched(ctx context.Context, dispatched []<-chan struct{}) error {
	for _, done := range dispatched {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (o *Orchestrator) eligibilityState() eligibilityState {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.eligibilityStateLocked("")
}

func (o *Orchestrator) eligibilityStateLocked(ignoreClaimIssueID string) eligibilityState {
	running := make(map[string]runningIssue, len(o.snapshot.Running))
	for _, entry := range o.snapshot.Running {
		running[entry.IssueID] = runningIssue{state: entry.State}
	}
	claimed := make(map[string]bool, len(o.claimed))
	for id, value := range o.claimed {
		if id == ignoreClaimIssueID {
			continue
		}
		claimed[id] = value
	}
	policy := effectiveReviewPolicy(o.opts.Workflow.Config.Agent)
	return eligibilityState{
		activeStates:   o.opts.Workflow.Config.Tracker.ActiveStates,
		terminalStates: o.opts.Workflow.Config.Tracker.TerminalStates,
		claimed:        claimed,
		running:        running,
		maxConcurrent:  maxConcurrentAgents(o.opts),
		perState:       o.opts.Workflow.Config.Agent.MaxConcurrentAgentsByState,
		aiReview:       policy.allowsAIReviewState(),
	}
}

func (o *Orchestrator) globalSlotsAvailable() int {
	o.mu.RLock()
	defer o.mu.RUnlock()

	available := maxConcurrentAgents(o.opts) - len(o.snapshot.Running)
	if available < 0 {
		return 0
	}
	return available
}

func (o *Orchestrator) claimIssue(issue types.Issue) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.claimed[issue.ID] = true
}

func (o *Orchestrator) releaseIssue(issueID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.claimed, issueID)
}

func (o *Orchestrator) setServiceContext(ctx context.Context) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.serviceCtx = ctx
}

func (o *Orchestrator) serviceContext() context.Context {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.serviceCtx != nil {
		return o.serviceCtx
	}
	return context.Background()
}

func (o *Orchestrator) serviceDoneLocked() bool {
	if o.serviceCtx == nil {
		return false
	}
	select {
	case <-o.serviceCtx.Done():
		return true
	default:
		return false
	}
}

func (o *Orchestrator) serviceDone() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.serviceDoneLocked()
}

func (o *Orchestrator) currentOptions() Options {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.opts
}

func (o *Orchestrator) currentRuntime() runtimeSnapshot {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return runtimeSnapshot{
		workflow:    o.opts.Workflow,
		tracker:     o.opts.Tracker,
		workspace:   o.opts.Workspace,
		runner:      o.opts.Runner,
		repoRoot:    o.opts.RepoRoot,
		mergeTarget: o.opts.MergeTarget,
	}
}

func (o *Orchestrator) refreshWorkflow() bool {
	if o.opts.Reloader == nil {
		return false
	}
	loaded, changed, err := o.opts.Reloader.ReloadIfChanged()
	if err != nil {
		o.setLastError(err.Error())
		o.log("", "workflow_reload_failed", err.Error(), nil)
		return true
	}
	if changed {
		tracker, workspaceManager, runner, err := o.reloadDependencies(loaded)
		if err != nil {
			o.setLastError(err.Error())
			o.log("", "workflow_reload_failed", err.Error(), nil)
			return true
		}
		o.mu.Lock()
		o.opts.Reloader.CommitCandidate()
		o.opts.Workflow = loaded
		o.opts.Tracker = tracker
		o.opts.Workspace = workspaceManager
		o.opts.Runner = runner
		o.snapshot.Polling.IntervalMS = o.opts.Workflow.Config.Polling.IntervalMS
		o.mu.Unlock()
		o.clearLastError()
		o.log("", "workflow_reloaded", "workflow reload completed", nil)
	}
	return false
}

func (o *Orchestrator) reloadDependencies(loaded *types.Workflow) (Tracker, *workspace.Manager, AgentRunner, error) {
	opts := o.currentOptions()
	tracker := opts.Tracker
	if opts.TrackerFactory != nil {
		next, err := opts.TrackerFactory(loaded.Config.Tracker)
		if err != nil {
			return nil, nil, nil, err
		}
		tracker = next
	}
	workspaceManager := opts.Workspace
	if opts.WorkspaceFactory != nil {
		workspaceManager = opts.WorkspaceFactory(loaded.Config.Workspace, loaded.Config.Hooks)
	}
	o.configureWorkspaceObserver(workspaceManager)
	runner := opts.Runner
	if opts.RunnerFactory != nil {
		runner = opts.RunnerFactory(loaded.Config.Codex)
	}
	return tracker, workspaceManager, runner, nil
}

func (o *Orchestrator) configureWorkspaceObserver(manager *workspace.Manager) {
	if manager == nil {
		return
	}
	manager.SetHookObserver(o.logWorkspaceHook)
}

func (o *Orchestrator) logWorkspaceHook(event workspace.HookEvent) {
	fields := map[string]any{
		"hook":           event.Name,
		"stage":          event.Stage,
		"workspace_path": event.CWD,
		"command":        logPreview(event.Script, 240),
	}
	if event.Source != "" {
		fields["source"] = event.Source
	}
	if event.Duration > 0 {
		fields["duration_ms"] = event.Duration.Milliseconds()
	}
	if output := logPreview(event.Output, 200); output != "" {
		fields["output"] = output
	}
	if event.Err != nil {
		fields["error"] = logPreview(event.Err.Error(), 200)
	}
	issue := types.Issue{
		ID:         event.IssueID,
		Identifier: event.IssueIdentifier,
	}
	if issue.Identifier == "" && event.CWD != "" {
		issue.Identifier = filepath.Base(event.CWD)
	}
	logEvent := "workspace_hook_" + event.Stage
	if event.Stage == "failed" || event.Stage == "timed_out" {
		logEvent = "workspace_hook_failed"
	}
	message := fmt.Sprintf("%s hook %s", event.Name, event.Stage)
	if issue.Identifier != "" || issue.ID != "" {
		o.logIssue(issue, logEvent, message, fields)
		return
	}
	o.log("", logEvent, message, fields)
}

func (o *Orchestrator) dispatchIssue(ctx context.Context, issue types.Issue, attempt int) bool {
	_, ok := o.dispatchIssueDone(ctx, issue, attempt)
	return ok
}

func (o *Orchestrator) dispatchIssueDone(ctx context.Context, issue types.Issue, attempt int) (<-chan struct{}, bool) {
	o.mu.Lock()
	if o.serviceDoneLocked() {
		o.mu.Unlock()
		return nil, false
	}
	if o.runningCancel[issue.ID] != nil {
		o.mu.Unlock()
		return nil, false
	}
	state := o.eligibilityStateLocked(issue.ID)
	ok, reason := candidateEligible(issue, state)
	if !ok && reason != "claimed" {
		o.mu.Unlock()
		return nil, false
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	rt := runtimeSnapshot{
		workflow:    o.opts.Workflow,
		tracker:     o.opts.Tracker,
		workspace:   o.opts.Workspace,
		runner:      o.opts.Runner,
		repoRoot:    o.opts.RepoRoot,
		mergeTarget: o.opts.MergeTarget,
	}
	o.claimed[issue.ID] = true
	o.runningCancel[issue.ID] = cancel
	if old := o.retryTimers[issue.ID]; old != nil {
		old.Stop()
	}
	delete(o.retryTimers, issue.ID)
	delete(o.retryAttempts, issue.ID)
	o.removeRetryLocked(issue.ID)
	o.setRunningLocked(observability.RunningEntry{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		State:           issue.State,
		StartedAt:       time.Now(),
		LastEvent:       "queued",
		LastMessage:     "queued for worker",
	})
	o.mu.Unlock()

	go func() {
		defer close(done)
		err := o.runAgentWith(workerCtx, rt, issue, attempt)
		o.workerExited(rt, issue, attempt, err)
	}()
	return done, true
}

func (o *Orchestrator) workerExited(rt runtimeSnapshot, issue types.Issue, attempt int, err error) {
	o.removeRunning(issue.ID)
	o.mu.Lock()
	if cancel := o.runningCancel[issue.ID]; cancel != nil {
		cancel()
	}
	delete(o.runningCancel, issue.ID)
	o.mu.Unlock()

	if cleanup, ok := o.popPendingTerminalCleanup(issue.ID); ok {
		o.cleanupWorkspace(context.Background(), rt.workspace, cleanup.issue, cleanup.entry)
		o.releaseIssue(issue.ID)
		return
	}
	if errors.Is(err, context.Canceled) {
		if !o.retryQueued(issue.ID) {
			o.releaseIssue(issue.ID)
		}
		return
	}
	if errors.Is(err, errNoRetryNeeded) {
		o.releaseIssue(issue.ID)
		o.signalPollNow()
		return
	}
	if o.serviceDone() {
		o.releaseIssue(issue.ID)
		return
	}
	if err != nil {
		o.logIssue(issue, "issue_error", err.Error(), nil)
		o.scheduleRetry(issue, attempt+1, retryFailure, err)
		return
	}
	o.scheduleRetry(issue, 1, retryContinuation, nil)
}

func (o *Orchestrator) handleRetry(issueID string) {
	ctx := o.serviceContext()
	if err := ctx.Err(); err != nil {
		o.clearRetry(issueID)
		o.releaseIssue(issueID)
		o.signalPollNow()
		return
	}
	rt := o.currentRuntime()
	issues, err := rt.tracker.FetchActiveIssues(ctx, rt.workflow.Config.Tracker.ActiveStates)
	if err != nil {
		wrapped := fmt.Errorf("retry poll failed: %w", err)
		o.setLastError(wrapped.Error())
		o.log("", "retry_fetch_error", wrapped.Error(), map[string]any{"issue_id": issueID})
		if issue, attempt, ok := o.retryIssue(issueID); ok {
			o.scheduleRetry(issue, attempt+1, retryFailure, wrapped)
		}
		o.signalPollNow()
		return
	}

	var issue types.Issue
	found := false
	for _, candidate := range issues {
		if candidate.ID == issueID {
			issue = candidate
			found = true
			break
		}
	}
	if !found {
		o.clearRetry(issueID)
		o.releaseIssue(issueID)
		o.signalPollNow()
		return
	}

	state := o.eligibilityState()
	delete(state.claimed, issueID)
	ok, reason := candidateEligible(issue, state)
	if !ok {
		if reason == "no_available_orchestrator_slots" {
			o.mu.RLock()
			attempt := o.retryAttempts[issueID]
			o.mu.RUnlock()
			o.scheduleRetry(issue, attempt+1, retryFailure, errors.New("no available orchestrator slots"))
			o.signalPollNow()
			return
		}
		if reason == "waiting_for_review" {
			o.logIssue(issue, "waiting_for_review", "issue is waiting for human review", nil)
		}
		o.clearRetry(issueID)
		o.releaseIssue(issueID)
		o.signalPollNow()
		return
	}

	o.mu.Lock()
	attempt := o.retryAttempts[issueID]
	o.mu.Unlock()

	if !o.dispatchIssue(ctx, issue, attempt) {
		o.scheduleRetry(issue, attempt+1, retryFailure, errors.New("no available orchestrator slots"))
		o.signalPollNow()
		return
	}
	o.signalPollNow()
}

func (o *Orchestrator) retryIssue(issueID string) (types.Issue, int, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	attempt := o.retryAttempts[issueID]
	for _, entry := range o.snapshot.Retrying {
		if entry.IssueID == issueID {
			if attempt < 1 {
				attempt = entry.Attempt
			}
			return types.Issue{
				ID:         entry.IssueID,
				Identifier: entry.IssueIdentifier,
			}, attempt, true
		}
	}
	return types.Issue{}, 0, false
}

func (o *Orchestrator) signalPollNow() {
	select {
	case o.pollNow <- struct{}{}:
	default:
	}
}

func (o *Orchestrator) handleIssue(ctx context.Context, issue types.Issue) error {
	return o.runAgent(ctx, issue, 0)
}

func (o *Orchestrator) runAgent(ctx context.Context, issue types.Issue, attempt int) error {
	err := o.runAgentWith(ctx, o.currentRuntime(), issue, attempt)
	if errors.Is(err, errNoRetryNeeded) {
		return nil
	}
	return err
}

func (o *Orchestrator) runAgentWith(ctx context.Context, rt runtimeSnapshot, issue types.Issue, attempt int) error {
	if isTerminal(issue.State, rt.workflow.Config.Tracker.TerminalStates) {
		return nil
	}
	switch issue.State {
	case "Todo":
		if err := rt.tracker.UpdateIssueState(ctx, issue.ID, "In Progress"); err != nil {
			return err
		}
		issue.State = "In Progress"
		o.logIssue(issue, "state_changed", "Todo -> In Progress", nil)
	}
	if skill := effectiveStateSkill(rt.workflow.Config.Agent, issue.State); skill.path != "" {
		if err := o.runStateSkill(ctx, rt, issue, skill); err != nil {
			return err
		}
		return errNoRetryNeeded
	}
	switch issue.State {
	case "Human Review", "In Review":
		o.logIssue(issue, "waiting_for_review", "issue is waiting for human review", nil)
		return errNoRetryNeeded
	case "AI Review":
		return o.reviewIssueState(ctx, rt, issue)
	}
	o.setRunning(observability.RunningEntry{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		State:           issue.State,
		TurnCount:       1,
		StartedAt:       time.Now(),
		LastEvent:       "preparing workspace",
		LastMessage:     "preparing workspace",
	})
	hookCtx := workspace.WithHookIssue(ctx, issue)
	workspacePath, _, err := rt.workspace.Ensure(hookCtx, issue)
	if err != nil {
		o.removeRunning(issue.ID)
		return err
	}
	defer func() {
		if err := rt.workspace.AfterRun(workspace.WithHookIssue(context.Background(), issue), workspacePath); err != nil {
			o.logIssue(issue, "after_run_hook_failed", err.Error(), nil)
		}
	}()
	if err := rt.workspace.BeforeRun(hookCtx, workspacePath); err != nil {
		o.removeRunning(issue.ID)
		return err
	}
	if err := o.upsertWorkpad(ctx, rt, issue, "initial", initialWorkpad(issue, workspacePath)); err != nil {
		o.removeRunning(issue.ID)
		return err
	}
	baseHead, _ := gitOutput(ctx, workspacePath, "rev-parse", "--short", "HEAD")
	maxTurns := rt.workflow.Config.Agent.MaxTurns
	var renderAttempt *int
	if attempt > 0 {
		value := attempt
		renderAttempt = &value
	}
	prompt, err := workflow.Render(rt.workflow.PromptTemplate, issue, renderAttempt)
	if err != nil {
		return err
	}
	o.setRunning(observability.RunningEntry{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		State:           issue.State,
		WorkspacePath:   workspacePath,
		TurnCount:       1,
		StartedAt:       time.Now(),
	})
	var nextIssue *types.Issue
	maxTurnsReached := false
	request := codex.SessionRequest{
		WorkspacePath: workspacePath,
		Issue:         issue,
		Prompts:       []codex.TurnPrompt{{Text: prompt, Attempt: renderAttempt}},
	}
	request.AfterTurn = func(ctx context.Context, result codex.Result, turn int) (codex.TurnPrompt, bool, error) {
		o.logIssue(issue, "turn_completed", "Codex turn completed", map[string]any{"session_id": result.SessionID})
		nextState, handoff, handoffErr := o.handoffAfterTurn(ctx, rt, issue, workspacePath, strings.TrimSpace(baseHead), result)
		if handoffErr != nil {
			return codex.TurnPrompt{}, false, handoffErr
		}
		if handoff {
			updated := issue
			updated.State = nextState
			nextIssue = &updated
			return codex.TurnPrompt{}, false, nil
		}
		refreshed, err := rt.tracker.FetchIssue(ctx, issue.ID)
		if err != nil {
			return codex.TurnPrompt{}, false, err
		}
		if !isActive(refreshed.State, rt.workflow.Config.Tracker.ActiveStates) || isTerminal(refreshed.State, rt.workflow.Config.Tracker.TerminalStates) {
			return codex.TurnPrompt{}, false, nil
		}
		if refreshed.State == "Human Review" || refreshed.State == "In Review" || refreshed.State == "Merging" {
			nextIssue = &refreshed
			return codex.TurnPrompt{}, false, nil
		}
		if turn >= maxTurns {
			maxTurnsReached = true
			return codex.TurnPrompt{}, false, nil
		}
		issue = refreshed
		o.setRunning(observability.RunningEntry{
			IssueID:         issue.ID,
			IssueIdentifier: issue.Identifier,
			State:           issue.State,
			WorkspacePath:   workspacePath,
			TurnCount:       turn + 1,
			StartedAt:       time.Now(),
		})
		return codex.TurnPrompt{Text: continuationPromptText, Continuation: true}, true, nil
	}
	_, err = rt.runner.RunSession(ctx, request, func(event codex.Event) {
		o.updateRunningFromEvent(issue.ID, event)
		o.logIssue(issue, "codex_event", event.Name, event.Payload)
	})
	o.removeRunning(issue.ID)
	if err != nil {
		_ = o.upsertWorkpad(context.Background(), rt, issue, "blocked", blockedWorkpad(issue, workspacePath, err))
		return err
	}
	if nextIssue != nil {
		if nextIssue.State == "Human Review" || nextIssue.State == "In Review" {
			o.logIssue(*nextIssue, "waiting_for_review", "issue is waiting for human review", nil)
			return errNoRetryNeeded
		}
		if nextIssue.State == "AI Review" || nextIssue.State == "Rework" {
			return errNoRetryNeeded
		}
		return o.runAgentWith(ctx, rt, *nextIssue, attempt)
	}
	if maxTurnsReached {
		return fmt.Errorf("reached max turns for %s while issue stayed active", issue.Identifier)
	}
	return nil
}

func (o *Orchestrator) handoffAfterTurn(ctx context.Context, rt runtimeSnapshot, issue types.Issue, workspacePath, baseHead string, result codex.Result) (string, bool, error) {
	head, err := gitOutput(ctx, workspacePath, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", false, fmt.Errorf("read HEAD after Codex turn: %w", err)
	}
	head = strings.TrimSpace(head)
	if head == "" || head == baseHead {
		return "", false, nil
	}

	changedFiles, _ := gitOutput(ctx, workspacePath, "show", "--name-only", "--format=", "--no-renames", "HEAD")
	status, _ := gitOutput(ctx, workspacePath, "status", "--short")
	workpad := handoffWorkpad(issue, workspacePath, baseHead, head, result, changedFiles, status)
	if err := o.upsertWorkpad(ctx, rt, issue, "handoff", workpad); err != nil {
		return "", false, err
	}
	policy := effectiveReviewPolicy(rt.workflow.Config.Agent)
	if policy.runsAIReviewAfterCommit() {
		if err := rt.tracker.UpdateIssueState(ctx, issue.ID, "AI Review"); err != nil {
			return "", false, err
		}
		outcome := o.aiReviewAfterCommit(ctx, policy, issue, workspacePath, baseHead, head, changedFiles, status)
		if err := o.upsertWorkpad(ctx, rt, issue, "ai_review", aiReviewWorkpad(issue, workspacePath, baseHead, head, outcome)); err != nil {
			return "", false, err
		}
		o.logIssue(issue, "state_changed", "In Progress -> AI Review", map[string]any{
			"commit":        head,
			"changed_files": outcome.ChangedFiles,
		})
		if outcome.Passed {
			nextState := policy.stateAfterPassedAIReview()
			if err := rt.tracker.UpdateIssueState(ctx, issue.ID, nextState); err != nil {
				return "", false, err
			}
			o.logIssue(issue, "ai_review_completed", "AI Review passed", map[string]any{"commit": head})
			o.logIssue(issue, "state_changed", "AI Review -> "+nextState, map[string]any{"commit": head})
			return nextState, true, nil
		} else if policy.reworkOnAIFail() {
			if err := rt.tracker.UpdateIssueState(ctx, issue.ID, "Rework"); err != nil {
				return "", false, err
			}
			o.logIssue(issue, "ai_review_failed", outcome.Summary, map[string]any{"reasons": outcome.Reasons})
			o.logIssue(issue, "state_changed", "AI Review -> Rework", map[string]any{"commit": head})
			return "Rework", true, nil
		} else {
			o.logIssue(issue, "ai_review_completed", outcome.Summary, map[string]any{"commit": head, "reasons": outcome.Reasons})
		}
		return "AI Review", true, nil
	}
	if err := rt.tracker.UpdateIssueState(ctx, issue.ID, "Human Review"); err != nil {
		return "", false, err
	}
	o.logIssue(issue, "state_changed", "In Progress -> Human Review", map[string]any{
		"commit":        head,
		"changed_files": nonEmptyLines(changedFiles),
	})
	return "Human Review", true, nil
}

func (o *Orchestrator) reviewIssueState(ctx context.Context, rt runtimeSnapshot, issue types.Issue) error {
	policy := effectiveReviewPolicy(rt.workflow.Config.Agent)
	if !policy.allowsAIReviewState() {
		o.logIssue(issue, "waiting_for_ai_review", "AI Review is disabled in workflow config", nil)
		return errNoRetryNeeded
	}
	workspacePath, _, err := rt.workspace.Ensure(ctx, issue)
	if err != nil {
		return err
	}
	head, err := gitOutput(ctx, workspacePath, "rev-parse", "--short", "HEAD")
	if err != nil {
		return fmt.Errorf("read HEAD for AI Review: %w", err)
	}
	baseHead, err := gitOutput(ctx, workspacePath, "rev-parse", "--short", "HEAD~1")
	if err != nil {
		baseHead = ""
	}
	changedFiles, _ := gitOutput(ctx, workspacePath, "show", "--name-only", "--format=", "--no-renames", "HEAD")
	status, _ := gitOutput(ctx, workspacePath, "status", "--short")
	outcome := o.aiReviewAfterCommit(ctx, policy, issue, workspacePath, strings.TrimSpace(baseHead), strings.TrimSpace(head), changedFiles, status)
	if err := o.upsertWorkpad(ctx, rt, issue, "ai_review", aiReviewWorkpad(issue, workspacePath, strings.TrimSpace(baseHead), strings.TrimSpace(head), outcome)); err != nil {
		return err
	}
	if outcome.Passed {
		nextState := policy.stateAfterPassedAIReview()
		if err := rt.tracker.UpdateIssueState(ctx, issue.ID, nextState); err != nil {
			return err
		}
		o.logIssue(issue, "ai_review_completed", "AI Review passed", map[string]any{"commit": strings.TrimSpace(head)})
		o.logIssue(issue, "state_changed", "AI Review -> "+nextState, map[string]any{"commit": strings.TrimSpace(head)})
		if nextState == "Merging" {
			issue.State = "Merging"
			return o.runStateSkill(ctx, rt, issue, effectiveStateSkill(rt.workflow.Config.Agent, issue.State))
		}
		return errNoRetryNeeded
	}
	if policy.reworkOnAIFail() {
		if err := rt.tracker.UpdateIssueState(ctx, issue.ID, "Rework"); err != nil {
			return err
		}
		o.logIssue(issue, "ai_review_failed", outcome.Summary, map[string]any{"reasons": outcome.Reasons})
		o.logIssue(issue, "state_changed", "AI Review -> Rework", map[string]any{"commit": strings.TrimSpace(head)})
		return errNoRetryNeeded
	}
	o.logIssue(issue, "ai_review_completed", outcome.Summary, map[string]any{"commit": strings.TrimSpace(head), "reasons": outcome.Reasons})
	return errNoRetryNeeded
}

const (
	reviewPolicyHuman = "human"
	reviewPolicyAI    = "ai"
	reviewPolicyAuto  = "auto"
	aiFailRework      = "rework"
	aiFailHold        = "hold"
	defaultMergeSkill = ".codex/skills/local-merge/SKILL.md"
)

type reviewPolicy struct {
	mode                 string
	allowManualAIReview  bool
	onAIFail             string
	expectedChangedFiles []string
}

type stateSkill struct {
	state string
	path  string
}

func effectiveReviewPolicy(agent types.AgentConfig) reviewPolicy {
	cfg := agent.ReviewPolicy
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		return legacyReviewPolicy(agent.AIReview)
	}
	if mode != reviewPolicyAI && mode != reviewPolicyAuto {
		mode = reviewPolicyHuman
	}
	onFail := strings.ToLower(strings.TrimSpace(cfg.OnAIFail))
	if onFail == "" {
		onFail = aiFailRework
	}
	if onFail != aiFailRework {
		onFail = aiFailHold
	}
	expected := cfg.ExpectedChangedFiles
	if len(expected) == 0 {
		expected = agent.AIReview.ExpectedChangedFiles
	}
	return reviewPolicy{
		mode:                 mode,
		allowManualAIReview:  cfg.AllowManualAIReview,
		onAIFail:             onFail,
		expectedChangedFiles: expected,
	}
}

func legacyReviewPolicy(review types.AIReviewConfig) reviewPolicy {
	policy := reviewPolicy{
		mode:     reviewPolicyHuman,
		onAIFail: aiFailRework,
	}
	if review.Enabled {
		policy.mode = reviewPolicyAI
		if review.AutoMerge {
			policy.mode = reviewPolicyAuto
		}
		if !review.ReworkOnFailure {
			policy.onAIFail = aiFailHold
		}
		policy.expectedChangedFiles = review.ExpectedChangedFiles
	}
	return policy
}

func (p reviewPolicy) allowsAIReviewState() bool {
	return p.mode == reviewPolicyAI || p.mode == reviewPolicyAuto || p.allowManualAIReview
}

func (p reviewPolicy) runsAIReviewAfterCommit() bool {
	return p.mode == reviewPolicyAI || p.mode == reviewPolicyAuto
}

func (p reviewPolicy) stateAfterPassedAIReview() string {
	if p.mode == reviewPolicyAuto {
		return "Merging"
	}
	return "Human Review"
}

func (p reviewPolicy) reworkOnAIFail() bool {
	return p.onAIFail == aiFailRework
}

func (o *Orchestrator) runStateSkill(ctx context.Context, rt runtimeSnapshot, issue types.Issue, skill stateSkill) error {
	if rt.runner == nil {
		return fmt.Errorf("%s skill %s requires an agent runner", skill.state, skill.path)
	}
	hookCtx := workspace.WithHookIssue(ctx, issue)
	workspacePath, _, err := rt.workspace.Ensure(hookCtx, issue)
	if err != nil {
		return err
	}
	if err := rt.workspace.BeforeRun(hookCtx, workspacePath); err != nil {
		return err
	}
	defer func() {
		if err := rt.workspace.AfterRun(workspace.WithHookIssue(context.Background(), issue), workspacePath); err != nil {
			o.logIssue(issue, "after_run_hook_failed", err.Error(), nil)
		}
	}()
	repoRoot := rt.repoRoot
	if repoRoot == "" {
		repoRoot = "."
	}
	target := rt.mergeTarget
	if target == "" {
		target = "main"
	}
	started := time.Now()
	o.logIssue(issue, "merge_skill_started", "starting merge skill", map[string]any{
		"state":     skill.state,
		"skill":     skill.path,
		"target":    target,
		"workspace": workspacePath,
	})
	result, err := rt.runner.RunSession(ctx, codex.SessionRequest{
		WorkspacePath: workspacePath,
		Issue:         issue,
		Prompts: []codex.TurnPrompt{{
			Text: stateSkillPrompt(issue, skill, repoRoot, workspacePath, target),
		}},
	}, func(event codex.Event) {
		o.updateRunningFromEvent(issue.ID, event)
		o.logIssue(issue, "codex_event", event.Name, event.Payload)
	})
	if err != nil {
		_ = o.upsertWorkpad(ctx, rt, issue, "merge_blocked", stateSkillBlockedWorkpad(issue, skill, err.Error()))
		return fmt.Errorf("%s skill %s failed: %w", skill.state, skill.path, err)
	}
	duration := time.Since(started)
	if err := o.upsertWorkpad(ctx, rt, issue, "merge_completed", stateSkillCompletedWorkpad(issue, skill, workspacePath, target, duration, result.SessionID)); err != nil {
		return err
	}
	o.logIssue(issue, "merge_skill_completed", "merge skill completed", map[string]any{
		"state":       skill.state,
		"skill":       skill.path,
		"target":      target,
		"session_id":  result.SessionID,
		"duration_ms": duration.Milliseconds(),
	})
	if strings.EqualFold(issue.State, "Merging") {
		if err := rt.tracker.UpdateIssueState(ctx, issue.ID, "Done"); err != nil {
			return err
		}
		o.logIssue(issue, "state_changed", "Merging -> Done", map[string]any{"skill": skill.path, "target": target})
	}
	return nil
}

func effectiveStateSkill(agent types.AgentConfig, state string) stateSkill {
	path := configuredStateSkillPath(agent, state)
	if path == "" && strings.EqualFold(state, "Merging") {
		path = strings.TrimSpace(agent.MergePolicy.Skill)
	}
	if path == "" && strings.EqualFold(state, "Merging") {
		path = defaultMergeSkill
	}
	return stateSkill{state: state, path: path}
}

func configuredStateSkillPath(agent types.AgentConfig, state string) string {
	for key, value := range agent.StateSkills {
		if strings.EqualFold(strings.TrimSpace(key), state) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func absoluteSkillPath(repoRoot, skillPath string) string {
	skillPath = strings.TrimSpace(skillPath)
	if skillPath == "" {
		skillPath = defaultMergeSkill
	}
	return filepath.Clean(filepath.Join(repoRoot, skillPath))
}

func stateSkillPrompt(issue types.Issue, skill stateSkill, repoRoot, workspacePath, target string) string {
	absolutePath := absoluteSkillPath(repoRoot, skill.path)
	return fmt.Sprintf(`你正在处理 Linear ticket %s 的 %s 阶段。

这是状态驱动的 skill 阶段。请按当前配置的 repo-root 相对路径执行对应 skill，不要在 Go 代码语义之外发明固定流程。

- state: %s
- skill path: %s
- skill 文件（以 repo root 为准）: %s
- repo root: %s
- issue worktree: %s
- merge target: %s

要求：
1. 先打开并遵循 repo root 里的 skill 文件，不要改用其他流程；如果 issue worktree 里有旧版同名 skill，以 repo root 版本为准。
2. 不要调用 Linear MCP/app tool；无人值守写 Linear 时必须使用 linear CLI，或者只把结果写入最终回复交给父 orchestrator 记录。
3. 写给 Linear、GitHub 或 workpad 的新增可见内容使用中文。
4. 如果 skill 完成并已验证，把关键证据写入 workpad；如果阻塞，写清 blocker 和证据。
5. 不要越权清理或回退不属于本 issue 的本地改动。
`, issue.Identifier, skill.state, skill.state, skill.path, absolutePath, repoRoot, workspacePath, target)
}

func nonEmptyLines(value string) []string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

type aiReviewOutcome struct {
	Passed       bool
	Summary      string
	Reasons      []string
	ChangedFiles []string
}

func (o *Orchestrator) aiReviewAfterCommit(ctx context.Context, policy reviewPolicy, issue types.Issue, workspacePath, baseHead, head, changedFiles, status string) aiReviewOutcome {
	outcome := aiReviewOutcome{
		Passed:       true,
		Summary:      "AI Review 通过",
		ChangedFiles: nonEmptyLines(changedFiles),
	}
	if strings.TrimSpace(status) != "" {
		outcome.Passed = false
		outcome.Reasons = append(outcome.Reasons, "worktree 仍有未提交变更")
	}
	if baseHead != "" && head != "" {
		if _, err := gitOutput(ctx, workspacePath, "diff", "--check", baseHead+".."+head); err != nil {
			outcome.Passed = false
			outcome.Reasons = append(outcome.Reasons, "git diff --check 未通过: "+err.Error())
		}
	}
	expected := policy.expectedChangedFiles
	if len(expected) > 0 && !sameStringSet(outcome.ChangedFiles, expected) {
		outcome.Passed = false
		outcome.Reasons = append(outcome.Reasons, fmt.Sprintf("变更文件不符合预期: got %v want %v", outcome.ChangedFiles, expected))
	}
	if !outcome.Passed {
		outcome.Summary = "AI Review 未通过"
	}
	return outcome
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := map[string]int{}
	for _, item := range left {
		counts[item]++
	}
	for _, item := range right {
		if counts[item] == 0 {
			return false
		}
		counts[item]--
	}
	return true
}

func initialWorkpad(issue types.Issue, workspacePath string) string {
	return workflowWorkpad(issue, workpadProgress{Worktree: true}, fmt.Sprintf(`### 验收标准

- issue 按配置的本地流程推进。
- 本地 worktree 已创建：%s。

### 阶段记录

- %s 已准备好本地 worktree。
`, workspacePath, time.Now().Format(time.RFC3339)))
}

func blockedWorkpad(issue types.Issue, workspacePath string, err error) string {
	return workflowWorkpad(issue, workpadProgress{Worktree: true}, fmt.Sprintf(`### 验收标准

- issue 按配置的本地流程推进。
- 本地 worktree 已创建：%s。

### 阻塞项

- Agent 运行失败：%s
`, workspacePath, err))
}

func handoffWorkpad(issue types.Issue, workspacePath, baseHead, head string, result codex.Result, changedFiles, status string) string {
	return workflowWorkpad(issue, workpadProgress{Task: true, Worktree: true, Agent: true, Commit: true, Handoff: true}, fmt.Sprintf(`### 验收标准

- [x] issue 流程产生了本地提交。
- [x] 本地 worktree 路径：%s。
- [x] Go orchestrator 已通过 GraphQL 写入此 Linear Workpad 评论。

### 验证

- [x] 基准 HEAD：%s
- [x] 交接提交：%s
- [x] Codex session：%s

### 阶段记录

- %s Go orchestrator 检测到本地提交，并已将 issue 移动到 Review。
- 变更文件：

~~~text
%s
~~~

- 当前 git 状态：

~~~text
%s
~~~
`, workspacePath, baseHead, head, result.SessionID, time.Now().Format(time.RFC3339), strings.TrimSpace(changedFiles), strings.TrimSpace(status)))
}

func stateSkillBlockedWorkpad(issue types.Issue, skill stateSkill, output string) string {
	return workflowWorkpad(issue, workpadProgress{Task: true, Worktree: true, Agent: true, Commit: true, Handoff: true, Review: true, Merging: true}, fmt.Sprintf(`### 阻塞项

- %s 的 %s skill 执行失败。

### 指标

- skill：%s

### 阶段记录

~~~text
%s
~~~
`, issue.Identifier, skill.state, skill.path, output))
}

func aiReviewWorkpad(issue types.Issue, workspacePath, baseHead, head string, outcome aiReviewOutcome) string {
	return workflowWorkpad(issue, workpadProgress{Task: true, Worktree: true, Agent: true, Commit: true, Handoff: true, Review: true}, fmt.Sprintf(`### AI Review

- [x] %s 已生成本地提交。
- [x] Go orchestrator 已完成 AI Review。
- [x] 结论：%s

### 指标

- worktree：%s
- 基准 HEAD：%s
- 提交 HEAD：%s
- 变更文件：%s
- 原因：%s

### 记录

- %s AI Review 完成。
`, issue.Identifier, outcome.Summary, workspacePath, baseHead, head, strings.Join(outcome.ChangedFiles, ", "), strings.Join(outcome.Reasons, "; "), time.Now().Format(time.RFC3339)))
}

func stateSkillCompletedWorkpad(issue types.Issue, skill stateSkill, workspacePath, target string, duration time.Duration, sessionID string) string {
	return workflowWorkpad(issue, workpadProgress{Task: true, Worktree: true, Agent: true, Commit: true, Handoff: true, Review: true, Merging: true, Done: true}, fmt.Sprintf(`### 指标

- issue：%s
- state：%s
- skill：%s
- worktree：%s
- merge target：%s
- Codex session：%s
- skill 耗时：%dms

### 阶段记录

- %s skill 已完成。
`, issue.Identifier, skill.state, skill.path, workspacePath, target, sessionID, duration.Milliseconds(), skill.state))
}

type workpadProgress struct {
	Task     bool
	Worktree bool
	Agent    bool
	Commit   bool
	Handoff  bool
	Review   bool
	Merging  bool
	Done     bool
}

func workflowWorkpad(issue types.Issue, progress workpadProgress, body string) string {
	return fmt.Sprintf(`## Codex Workpad

### 任务计划

%s

### 框架进度

%s 准备 %s 的本地 worktree
%s 运行 Codex agent 完成 issue 任务
%s 检测本地提交和变更文件
%s 写入交接记录并流转到 Review
%s 完成 AI Review 或等待人工 review
%s 执行 Merging skill
%s 流转到 Done

%s`, taskPlanChecklist(issue, progress.Task), checkbox(progress.Worktree), issue.Identifier, checkbox(progress.Agent), checkbox(progress.Commit), checkbox(progress.Handoff), checkbox(progress.Review), checkbox(progress.Merging), checkbox(progress.Done), body)
}

func taskPlanChecklist(issue types.Issue, done bool) string {
	steps := taskPlanSteps(issue)
	lines := make([]string, 0, len(steps))
	for _, step := range steps {
		lines = append(lines, fmt.Sprintf("%s %s", checkbox(done), step))
	}
	return strings.Join(lines, "\n")
}

func taskPlanSteps(issue types.Issue) []string {
	var steps []string
	for _, line := range strings.Split(issue.Description, "\n") {
		step, ok := taskStepFromMarkdownLine(line)
		if !ok {
			continue
		}
		steps = append(steps, step)
		if len(steps) == 6 {
			break
		}
	}
	if len(steps) > 0 {
		return steps
	}
	title := strings.TrimSpace(issue.Title)
	if title == "" {
		title = issue.Identifier
	}
	return []string{"理解并完成 issue：" + title}
}

func taskStepFromMarkdownLine(line string) (string, bool) {
	item := strings.TrimSpace(line)
	foundListItem := false
	for _, prefix := range []string{"- [ ] ", "- [x] ", "- [X] ", "- ", "* "} {
		if strings.HasPrefix(item, prefix) {
			item = strings.TrimSpace(strings.TrimPrefix(item, prefix))
			foundListItem = true
			break
		}
	}
	if !foundListItem || item == "" || isConstraintOnlyStep(item) {
		return "", false
	}
	if strings.HasPrefix(item, "只改 ") {
		return "定位并只修改 " + strings.TrimSpace(strings.TrimPrefix(item, "只改 ")), true
	}
	return item, true
}

func isConstraintOnlyStep(item string) bool {
	for _, prefix := range []string{"不创建", "不 push", "不调用", "不要", "所有 Workpad", "全程"} {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func checkbox(done bool) string {
	if done {
		return "- [x]"
	}
	return "- [ ]"
}

func (o *Orchestrator) upsertWorkpad(ctx context.Context, rt runtimeSnapshot, issue types.Issue, phase, body string) error {
	if err := rt.tracker.UpsertWorkpad(ctx, issue.ID, body); err != nil {
		return err
	}
	fields := map[string]any{
		"phase": phase,
		"bytes": len([]byte(body)),
		"lines": countLines(body),
	}
	for key, value := range workpadLogSummary(body) {
		fields[key] = value
	}
	o.logIssue(issue, "workpad_updated", "Linear workpad updated", fields)
	return nil
}

func workpadLogSummary(body string) map[string]any {
	task := checklistCounter{}
	framework := checklistCounter{}
	section := ""
	sections := make([]string, 0)
	preview := make([]string, 0, 3)
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			section = strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))
			sections = append(sections, section)
			continue
		}
		done, text, ok := parseChecklistLine(trimmed)
		if !ok {
			if !inFence {
				preview = appendWorkpadPreview(preview, section, trimmed)
			}
			continue
		}
		switch section {
		case "任务计划":
			task.add(done, text)
		case "框架进度":
			framework.add(done, text)
		default:
			if !inFence {
				preview = appendWorkpadPreview(preview, section, text)
			}
		}
	}
	fields := map[string]any{}
	if len(sections) > 0 {
		fields["sections"] = strings.Join(sections, ",")
	}
	if task.total > 0 {
		fields["task_progress"] = task.progress()
	}
	if framework.total > 0 {
		fields["framework_progress"] = framework.progress()
	}
	if task.next != "" {
		fields["next_step"] = task.next
	} else if framework.next != "" {
		fields["next_step"] = framework.next
	}
	if len(preview) > 0 {
		fields["comment_preview"] = strings.Join(preview, "; ")
	}
	return fields
}

func appendWorkpadPreview(preview []string, section, line string) []string {
	if len(preview) >= 3 || section == "" || section == "任务计划" || section == "框架进度" {
		return preview
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
	if line == "" {
		return preview
	}
	return append(preview, truncateRunes(line, 120))
}

func logPreview(value string, max int) string {
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

type checklistCounter struct {
	done  int
	total int
	next  string
}

func (c *checklistCounter) add(done bool, text string) {
	c.total++
	if done {
		c.done++
		return
	}
	if c.next == "" {
		c.next = text
	}
}

func (c checklistCounter) progress() string {
	return fmt.Sprintf("%d/%d", c.done, c.total)
}

func parseChecklistLine(line string) (bool, string, bool) {
	switch {
	case strings.HasPrefix(line, "- [x] "):
		return true, strings.TrimSpace(strings.TrimPrefix(line, "- [x] ")), true
	case strings.HasPrefix(line, "- [X] "):
		return true, strings.TrimSpace(strings.TrimPrefix(line, "- [X] ")), true
	case strings.HasPrefix(line, "- [ ] "):
		return false, strings.TrimSpace(strings.TrimPrefix(line, "- [ ] ")), true
	default:
		return false, "", false
	}
}

func countLines(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}

func isActive(state string, active []string) bool {
	for _, item := range active {
		if item == state {
			return true
		}
	}
	return false
}

func isTerminal(state string, terminal []string) bool {
	for _, item := range terminal {
		if item == state {
			return true
		}
	}
	return false
}

func (o *Orchestrator) log(issue, event, message string, fields map[string]any) {
	if o.opts.Logger != nil {
		logEvent := logging.Event{
			Issue:           issue,
			IssueIdentifier: issue,
			Event:           event,
			Message:         message,
			Fields:          fields,
		}
		if id, _ := fields["issue_id"].(string); id != "" {
			logEvent.IssueID = id
		}
		if sessionID, _ := fields["session_id"].(string); sessionID != "" {
			logEvent.SessionID = sessionID
		}
		_ = o.opts.Logger.Write(logEvent)
	}
}

func (o *Orchestrator) logIssue(issue types.Issue, event, message string, fields map[string]any) {
	if o.opts.Logger == nil {
		return
	}
	fields = withLogField(fields, "issue_id", issue.ID)
	fields = withLogField(fields, "issue_identifier", issue.Identifier)
	logEvent := logging.Event{
		Issue:           issue.Identifier,
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		Event:           event,
		Message:         message,
		Fields:          fields,
	}
	if sessionID, _ := fields["session_id"].(string); sessionID != "" {
		logEvent.SessionID = sessionID
	}
	_ = o.opts.Logger.Write(logEvent)
}

func withLogField(fields map[string]any, key string, value any) map[string]any {
	if value == nil || value == "" {
		return fields
	}
	if fields == nil {
		fields = map[string]any{}
	}
	if _, ok := fields[key]; !ok {
		fields[key] = value
	}
	return fields
}

func pollingInterval(opts Options) time.Duration {
	if opts.Workflow != nil && opts.Workflow.Config.Polling.IntervalMS > 0 {
		return time.Duration(opts.Workflow.Config.Polling.IntervalMS) * time.Millisecond
	}
	return 5 * time.Second
}

func maxConcurrentAgents(opts Options) int {
	if opts.Workflow != nil && opts.Workflow.Config.Agent.MaxConcurrentAgents > 0 {
		return opts.Workflow.Config.Agent.MaxConcurrentAgents
	}
	return 10
}

func (o *Orchestrator) markPolling(checking bool, nextPollAt time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now()
	o.snapshot.Polling.Checking = checking
	if checking {
		o.snapshot.Polling.LastPollAt = now
	}
	o.snapshot.Polling.NextPollAt = nextPollAt
	o.snapshot.Polling.NextPollInMS = millisUntil(now, nextPollAt)
}

func (o *Orchestrator) setRunning(entry observability.RunningEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.setRunningLocked(entry)
}

func (o *Orchestrator) setRunningLocked(entry observability.RunningEntry) {
	for i, existing := range o.snapshot.Running {
		if existing.IssueID == entry.IssueID {
			o.snapshot.Running[i] = entry
			return
		}
	}
	o.snapshot.Running = append(o.snapshot.Running, entry)
}

func (o *Orchestrator) updateRunningFromEvent(issueID string, event codex.Event) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i := range o.snapshot.Running {
		if o.snapshot.Running[i].IssueID != issueID {
			continue
		}
		entry := &o.snapshot.Running[i]
		entry.LastEvent = codexEventName(event)
		entry.LastMessage = observability.HumanizeCodexEvent(event.Payload)
		entry.LastEventAt = time.Now()
		if sessionID, _ := event.Payload["session_id"].(string); sessionID != "" {
			entry.SessionID = sessionID
		}
		if pid, ok := numericPayloadInt(event.Payload["pid"]); ok {
			entry.PID = pid
		}
		if usage, ok := observability.ExtractTokenUsage(event.Payload); ok {
			delta := tokenDelta(usage, entry.Tokens)
			entry.Tokens = usage
			o.snapshot.CodexTotals.InputTokens += delta.InputTokens
			o.snapshot.CodexTotals.OutputTokens += delta.OutputTokens
			o.snapshot.CodexTotals.TotalTokens += delta.TotalTokens
		}
		if rateLimits, ok := observability.ExtractRateLimits(event.Payload); ok {
			o.snapshot.RateLimits = rateLimits
		}
		return
	}
}

func (o *Orchestrator) addRetry(entry observability.RetryEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i, existing := range o.snapshot.Retrying {
		if existing.IssueID == entry.IssueID {
			if entry.Attempt <= existing.Attempt {
				entry.Attempt = existing.Attempt + 1
			}
			o.snapshot.Retrying[i] = entry
			return
		}
	}
	o.snapshot.Retrying = append(o.snapshot.Retrying, entry)
}

func (o *Orchestrator) removeRetry(issueID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.removeRetryLocked(issueID)
}

func (o *Orchestrator) removeRetryLocked(issueID string) {
	retrying := o.snapshot.Retrying[:0]
	for _, entry := range o.snapshot.Retrying {
		if entry.IssueID != issueID {
			retrying = append(retrying, entry)
		}
	}
	o.snapshot.Retrying = retrying
}

func (o *Orchestrator) setLastError(message string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.snapshot.LastError = message
}

func (o *Orchestrator) clearLastError() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.snapshot.LastError = ""
}

func (o *Orchestrator) removeRunning(issueID string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now()
	running := o.snapshot.Running[:0]
	for _, entry := range o.snapshot.Running {
		if entry.IssueID == issueID {
			if !entry.StartedAt.IsZero() {
				o.snapshot.CodexTotals.SecondsRunning += now.Sub(entry.StartedAt).Seconds()
			}
			continue
		}
		running = append(running, entry)
	}
	o.snapshot.Running = running
}

func codexEventName(event codex.Event) string {
	if event.Name != "" {
		return event.Name
	}
	if method, _ := event.Payload["method"].(string); method != "" {
		return method
	}
	return "codex_event"
}

func tokenDelta(next, previous observability.TokenUsage) observability.TokenUsage {
	return observability.TokenUsage{
		InputTokens:  positiveDelta(next.InputTokens, previous.InputTokens),
		OutputTokens: positiveDelta(next.OutputTokens, previous.OutputTokens),
		TotalTokens:  positiveDelta(next.TotalTokens, previous.TotalTokens),
	}
}

func positiveDelta(next, previous int) int {
	if next <= previous {
		return 0
	}
	return next - previous
}

func numericPayloadInt(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

func millisUntil(now time.Time, target time.Time) int64 {
	if target.IsZero() || !target.After(now) {
		return 0
	}
	return target.Sub(now).Milliseconds()
}

func cloneRunning(entries []observability.RunningEntry) []observability.RunningEntry {
	clone := make([]observability.RunningEntry, len(entries))
	copy(clone, entries)
	return clone
}

func cloneRetrying(entries []observability.RetryEntry) []observability.RetryEntry {
	clone := make([]observability.RetryEntry, len(entries))
	copy(clone, entries)
	return clone
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, nested := range typed {
			clone[key] = cloneJSONValue(nested)
		}
		return clone
	case []any:
		clone := make([]any, len(typed))
		for i, nested := range typed {
			clone[i] = cloneJSONValue(nested)
		}
		return clone
	default:
		return typed
	}
}

func RepoRootFromWorkflow(workflowPath string) string {
	if workflowPath == "" {
		return "."
	}
	return filepath.Clean(filepath.Dir(workflowPath))
}

func NormalizeMergeTarget(target string) string {
	return strings.TrimSpace(target)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, string(output))
	}
	return string(output), nil
}
