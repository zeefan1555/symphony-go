package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	runtimeconfig "symphony-go/internal/runtime/config"
	"symphony-go/internal/runtime/logging"
	"symphony-go/internal/runtime/observability"
	"symphony-go/internal/runtime/telemetry"
	"symphony-go/internal/service/codex"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/issueflow"
	"symphony-go/internal/service/workspace"
)

type Tracker interface {
	FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error)
	FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error)
	FetchIssue(context.Context, string) (issuemodel.Issue, error)
	FetchIssueStatesByIDs(context.Context, []string) ([]issuemodel.Issue, error)
	UpdateIssueState(context.Context, string, string) error
}

type AgentRunner interface {
	RunSession(context.Context, codex.SessionRequest, func(codex.Event)) (codex.SessionResult, error)
}

type WorkflowReloader interface {
	Current() *runtimeconfig.Workflow
	ReloadIfChanged() (*runtimeconfig.Workflow, bool, error)
	CommitCandidate()
}

type Options struct {
	Workflow                 *runtimeconfig.Workflow
	Reloader                 WorkflowReloader
	Tracker                  Tracker
	TrackerFactory           func(runtimeconfig.TrackerConfig) (Tracker, error)
	Workspace                *workspace.Manager
	WorkspaceFactory         func(runtimeconfig.WorkspaceConfig, runtimeconfig.HooksConfig) *workspace.Manager
	Runner                   AgentRunner
	RunnerFactory            func(runtimeconfig.CodexConfig) AgentRunner
	WorkflowRunnerFactory    func(runtimeconfig.Config) (AgentRunner, error)
	RunnerFactoryWithTracker func(runtimeconfig.CodexConfig, Tracker) AgentRunner
	NewTimer                 func(time.Duration, func()) *time.Timer
	Logger                   *logging.Logger
	Telemetry                telemetry.Facade
	Once                     bool
	IssueFilter              string
	RepoRoot                 string
	MergeTarget              string
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
	workflow    *runtimeconfig.Workflow
	tracker     Tracker
	workspace   *workspace.Manager
	runner      AgentRunner
	telemetry   telemetry.Facade
	repoRoot    string
	mergeTarget string
}

func (rt runtimeSnapshot) issueFlowRuntime(observer issueflow.Observer) issueflow.Runtime {
	return issueflow.Runtime{
		Workflow:  rt.workflow,
		Tracker:   rt.tracker,
		Workspace: rt.workspace,
		Runner:    rt.runner,
		Observer:  observer,
		Telemetry: rt.telemetry,
	}
}

type terminalCleanup struct {
	issue issuemodel.Issue
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
	if reloadFailed {
		return nil, nil
	}
	issues, err := rt.tracker.FetchActiveIssues(ctx, rt.workflow.Config.Tracker.ActiveStates)
	if err != nil {
		o.setLastError(err.Error())
		return nil, err
	}
	o.clearLastError()
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
		refreshed, ok := o.revalidateCandidateForDispatch(ctx, rt, issue)
		if !ok {
			continue
		}
		issue = refreshed
		o.logIssue(issue, "dispatch_started", "dispatch started", map[string]any{"state": issue.State})
		done, ok := o.dispatchIssueDone(ctx, issue, 0)
		if !ok {
			o.logIssue(issue, "dispatch_skipped", "claimed", nil)
			continue
		}
		dispatched = append(dispatched, done)
	}
	return dispatched, nil
}

func (o *Orchestrator) revalidateCandidateForDispatch(ctx context.Context, rt runtimeSnapshot, issue issuemodel.Issue) (issuemodel.Issue, bool) {
	issues, err := rt.tracker.FetchIssueStatesByIDs(ctx, []string{issue.ID})
	if err != nil {
		o.logIssue(issue, "dispatch_skipped", "issue refresh failed before dispatch", map[string]any{"error": err.Error()})
		return issuemodel.Issue{}, false
	}
	if len(issues) == 0 {
		o.logIssue(issue, "dispatch_skipped", "issue disappeared before dispatch", nil)
		return issuemodel.Issue{}, false
	}
	refreshed, found := refreshedIssueByID(issues, issue.ID)
	if !found {
		o.logIssue(issue, "dispatch_skipped", "issue disappeared before dispatch", nil)
		return issuemodel.Issue{}, false
	}
	refreshed = mergeIssueRefresh(issue, refreshed)
	ok, reason := candidateEligible(refreshed, o.eligibilityState())
	if !ok {
		o.logIssue(refreshed, "dispatch_skipped", "stale candidate after issue refresh", map[string]any{"reason": reason})
		return issuemodel.Issue{}, false
	}
	return refreshed, true
}

func refreshedIssueByID(issues []issuemodel.Issue, id string) (issuemodel.Issue, bool) {
	for _, issue := range issues {
		if issue.ID == id {
			return issue, true
		}
	}
	return issuemodel.Issue{}, false
}

func mergeIssueRefresh(base, refreshed issuemodel.Issue) issuemodel.Issue {
	merged := base
	if refreshed.ID != "" {
		merged.ID = refreshed.ID
	}
	if refreshed.Identifier != "" {
		merged.Identifier = refreshed.Identifier
	}
	if refreshed.Title != "" {
		merged.Title = refreshed.Title
	}
	if refreshed.State != "" {
		merged.State = refreshed.State
	}
	if refreshed.BlockedBy != nil {
		merged.BlockedBy = refreshed.BlockedBy
	}
	return merged
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
			if shouldCancelRunningForTerminalState(issue.State) {
				o.cancelRunning(issue.ID)
			} else {
				o.updateRunningState(issue.ID, issue.State)
			}
			continue
		}
		if issue.AssignedToWorker != nil && !*issue.AssignedToWorker {
			o.logIssue(issue, "worker_routing_changed", "issue no longer assigned to this worker", map[string]any{"state": issue.State, "assignee_id": issue.AssigneeID})
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
		issue := issuemodel.Issue{
			ID:         entry.IssueID,
			Identifier: entry.IssueIdentifier,
			State:      entry.State,
		}
		err := fmt.Errorf("stalled after %s", timeout)
		o.scheduleRetry(issue, 1, retryFailure, err)
		o.cancelRunning(entry.IssueID)
		o.logIssue(issuemodel.Issue{ID: entry.IssueID, Identifier: entry.IssueIdentifier}, "worker_stalled", err.Error(), nil)
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

func (o *Orchestrator) markPendingTerminalCleanup(issue issuemodel.Issue, entry observability.RunningEntry) {
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

func (o *Orchestrator) cleanupWorkspace(ctx context.Context, manager *workspace.Manager, issue issuemodel.Issue, entry observability.RunningEntry) error {
	path := entry.WorkspacePath
	if path == "" {
		var err error
		path, err = manager.PathForIssue(issue)
		if err != nil {
			o.logIssue(issue, "workspace_cleanup_failed", err.Error(), nil)
			return err
		}
	}
	if err := manager.Remove(workspace.WithHookIssue(ctx, issue), path); err != nil {
		o.logIssue(issue, "workspace_cleanup_failed", err.Error(), map[string]any{"workspace_path": path})
		return err
	}
	if manager.StaticCWD() {
		o.logIssue(issue, "workspace_retained", "static cwd retained", map[string]any{"workspace_path": path})
		return nil
	}
	o.logIssue(issue, "workspace_cleaned", "workspace removed", map[string]any{"workspace_path": path})
	return nil
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

func (o *Orchestrator) claimIssue(issue issuemodel.Issue) {
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
		telemetry:   o.opts.Telemetry,
		repoRoot:    o.opts.RepoRoot,
		mergeTarget: effectiveMergeTarget(o.opts),
	}
}

func (o *Orchestrator) RuntimeConfig() runtimeconfig.Config {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.opts.Workflow == nil {
		return runtimeconfig.Config{}
	}
	return o.opts.Workflow.Config
}

func (o *Orchestrator) RuntimeWorkspace() *workspace.Manager {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.opts.Workspace
}

func (o *Orchestrator) RuntimeRunner() interface {
	RunSession(context.Context, codex.SessionRequest, func(codex.Event)) (codex.SessionResult, error)
} {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.opts.Runner
}

func (o *Orchestrator) RuntimeTracker() interface {
	FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error)
	FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error)
	FetchIssue(context.Context, string) (issuemodel.Issue, error)
	UpdateIssueState(context.Context, string, string) error
} {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.opts.Tracker
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

func (o *Orchestrator) reloadDependencies(loaded *runtimeconfig.Workflow) (Tracker, *workspace.Manager, AgentRunner, error) {
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
	if opts.RunnerFactoryWithTracker != nil {
		runner = opts.RunnerFactoryWithTracker(loaded.Config.Codex, tracker)
	} else if opts.WorkflowRunnerFactory != nil {
		next, err := opts.WorkflowRunnerFactory(loaded.Config)
		if err != nil {
			return nil, nil, nil, err
		}
		runner = next
	} else if opts.RunnerFactory != nil {
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
	issue := issuemodel.Issue{
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

func (o *Orchestrator) dispatchIssue(ctx context.Context, issue issuemodel.Issue, attempt int) bool {
	_, ok := o.dispatchIssueDone(ctx, issue, attempt)
	return ok
}

func (o *Orchestrator) dispatchIssueDone(ctx context.Context, issue issuemodel.Issue, attempt int) (<-chan struct{}, bool) {
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
		telemetry:   o.opts.Telemetry,
		repoRoot:    o.opts.RepoRoot,
		mergeTarget: effectiveMergeTarget(o.opts),
	}
	o.claimed[issue.ID] = true
	o.runningCancel[issue.ID] = cancel
	if old := o.retryTimers[issue.ID]; old != nil {
		old.Stop()
	}
	delete(o.retryTimers, issue.ID)
	delete(o.retryAttempts, issue.ID)
	o.removeRetryLocked(issue.ID)
	o.setRunningStageLocked(issue, attempt, "", issueflow.StageQueued, "queued for worker", "", 1)
	o.mu.Unlock()

	go func() {
		defer close(done)
		result, err := issueflow.RunIssueTrunk(workerCtx, rt.issueFlowRuntime(o), issue, attempt)
		o.workerExited(rt, issue, attempt, result, err)
	}()
	return done, true
}

func (o *Orchestrator) workerExited(rt runtimeSnapshot, issue issuemodel.Issue, attempt int, result issueflow.Result, err error) {
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
	if o.cleanupTerminalIssueAfterExit(context.Background(), rt, issue) {
		o.releaseIssue(issue.ID)
		o.signalPollNow()
		return
	}
	if errors.Is(err, context.Canceled) {
		if !o.retryQueued(issue.ID) {
			o.releaseIssue(issue.ID)
		}
		return
	}
	if result.Outcome == issueflow.OutcomeWaitHuman || result.Outcome == issueflow.OutcomeDone || result.Outcome == issueflow.OutcomeStopped {
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
	if result.Outcome == issueflow.OutcomeRetryContinuation {
		o.scheduleRetry(issue, 1, retryContinuation, nil)
		return
	}
	if result.Outcome == issueflow.OutcomeRetryFailure {
		o.scheduleRetry(issue, attempt+1, retryFailure, errors.New("issue flow requested failure retry"))
		return
	}
	o.releaseIssue(issue.ID)
	o.signalPollNow()
}

func (o *Orchestrator) cleanupTerminalIssueAfterExit(ctx context.Context, rt runtimeSnapshot, issue issuemodel.Issue) bool {
	refreshed, err := rt.tracker.FetchIssue(ctx, issue.ID)
	if err != nil {
		o.logIssue(issue, "terminal_cleanup_check_failed", err.Error(), nil)
		return false
	}
	if !isTerminal(refreshed.State, rt.workflow.Config.Tracker.TerminalStates) {
		return false
	}
	if refreshed.Identifier == "" {
		refreshed.Identifier = issue.Identifier
	}
	if err := o.cleanupWorkspace(workspace.WithHookSource(ctx, "worker_exit_terminal_cleanup"), rt.workspace, refreshed, observability.RunningEntry{}); err != nil {
		return false
	}
	return true
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

	var issue issuemodel.Issue
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

func (o *Orchestrator) retryIssue(issueID string) (issuemodel.Issue, int, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	attempt := o.retryAttempts[issueID]
	for _, entry := range o.snapshot.Retrying {
		if entry.IssueID == issueID {
			if attempt < 1 {
				attempt = entry.Attempt
			}
			return issuemodel.Issue{
				ID:         entry.IssueID,
				Identifier: entry.IssueIdentifier,
			}, attempt, true
		}
	}
	return issuemodel.Issue{}, 0, false
}

func (o *Orchestrator) signalPollNow() {
	_, _ = o.RequestRefresh(context.Background())
}

func (o *Orchestrator) RequestRefresh(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if o == nil {
		return false, nil
	}
	select {
	case o.pollNow <- struct{}{}:
		return true, nil
	default:
		return false, nil
	}
}

func (o *Orchestrator) handleIssue(ctx context.Context, issue issuemodel.Issue) error {
	return o.runAgent(ctx, issue, 0)
}

func (o *Orchestrator) runAgent(ctx context.Context, issue issuemodel.Issue, attempt int) error {
	_, err := issueflow.RunIssueTrunk(ctx, o.currentRuntime().issueFlowRuntime(o), issue, attempt)
	return err
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

func countLines(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}

func isActive(state string, active []string) bool {
	return stateNameIn(state, active)
}

func isTerminal(state string, terminal []string) bool {
	return stateNameIn(state, terminal)
}

func shouldCancelRunningForTerminalState(state string) bool {
	return !strings.EqualFold(strings.TrimSpace(state), "Done")
}

func (o *Orchestrator) log(issue, event, message string, fields map[string]any) {
	for key, value := range logging.SourceFields(1) {
		fields = withLogField(fields, key, value)
	}
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
	if o.opts.Logger != nil {
		_ = o.opts.Logger.Write(logEvent)
	}
	telemetry.RecordLog(context.Background(), o.opts.Telemetry, logEvent)
}

func (o *Orchestrator) logIssue(issue issuemodel.Issue, event, message string, fields map[string]any) {
	for key, value := range logging.SourceFields(1) {
		fields = withLogField(fields, key, value)
	}
	o.logIssueWithContext(context.Background(), issue, event, message, fields)
}

func (o *Orchestrator) logIssueWithContext(ctx context.Context, issue issuemodel.Issue, event, message string, fields map[string]any) {
	for key, value := range logging.SourceFields(1) {
		fields = withLogField(fields, key, value)
	}
	for key, value := range telemetry.TraceFields(ctx) {
		fields = withLogField(fields, key, value)
	}
	fields = withLogField(fields, "issue_id", issue.ID)
	fields = withLogField(fields, "issue_identifier", issue.Identifier)
	logEvent := logging.Event{
		Issue:           issue.Identifier,
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		TraceID:         stringField(fields, "trace_id"),
		SpanID:          stringField(fields, "span_id"),
		Event:           event,
		Message:         message,
		Fields:          fields,
	}
	if sessionID, _ := fields["session_id"].(string); sessionID != "" {
		logEvent.SessionID = sessionID
	}
	if o.opts.Logger != nil {
		_ = o.opts.Logger.Write(logEvent)
	}
	telemetry.RecordLog(ctx, o.opts.Telemetry, logEvent)
}

func (o *Orchestrator) LogIssue(ctx context.Context, issue issuemodel.Issue, event, message string, fields map[string]any) {
	o.logIssueWithContext(ctx, issue, event, message, fields)
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

func stringField(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	value, _ := fields[key].(string)
	return value
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
	telemetry.RecordIssueActive(context.Background(), o.opts.Telemetry, 1, runningMetricFields(entry))
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
		if sessionID := codexPayloadString(event.Payload, "session_id", "sessionId"); sessionID != "" {
			entry.SessionID = sessionID
		}
		if threadID := codexPayloadString(event.Payload, "thread_id", "threadId"); threadID != "" {
			entry.ThreadID = threadID
		}
		if turnID := codexPayloadString(event.Payload, "turn_id", "turnId"); turnID != "" {
			entry.TurnID = turnID
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
			telemetry.RecordCodexTokens(context.Background(), o.opts.Telemetry, delta.InputTokens, delta.OutputTokens, delta.TotalTokens, map[string]any{
				"phase": entry.AgentPhase,
				"stage": entry.Stage,
			})
		}
		if rateLimits, ok := observability.ExtractRateLimits(event.Payload); ok {
			o.snapshot.RateLimits = rateLimits
		}
		return
	}
}

func (o *Orchestrator) UpdateRunningFromEvent(issueID string, event codex.Event) {
	o.updateRunningFromEvent(issueID, event)
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
	telemetry.RecordIssueRetrying(context.Background(), o.opts.Telemetry, 1, retryMetricFields(entry))
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
			continue
		}
		telemetry.RecordIssueRetrying(context.Background(), o.opts.Telemetry, -1, retryMetricFields(entry))
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
			telemetry.RecordIssueActive(context.Background(), o.opts.Telemetry, -1, runningMetricFields(entry))
			continue
		}
		running = append(running, entry)
	}
	o.snapshot.Running = running
}

func (o *Orchestrator) RemoveRunning(issueID string) {
	o.removeRunning(issueID)
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

func runningMetricFields(entry observability.RunningEntry) map[string]any {
	return map[string]any{"stage": "active"}
}

func retryMetricFields(entry observability.RetryEntry) map[string]any {
	attemptKind := "retry"
	if entry.Attempt <= 1 {
		attemptKind = "first"
	}
	return map[string]any{
		"attempt_kind": attemptKind,
	}
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

func codexPayloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, _ := payload[key].(string); value != "" {
			return value
		}
	}
	params, _ := payload["params"].(map[string]any)
	for _, key := range keys {
		if value, _ := params[key].(string); value != "" {
			return value
		}
	}
	return ""
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

func effectiveMergeTarget(opts Options) string {
	target := NormalizeMergeTarget(opts.MergeTarget)
	if target == "" && opts.Workflow != nil {
		target = NormalizeMergeTarget(opts.Workflow.Config.Merge.Target)
	}
	if target == "" {
		return "main"
	}
	return target
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
