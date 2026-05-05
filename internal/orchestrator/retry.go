package orchestrator

import (
	"time"

	"github.com/zeefan1555/symphony-go/internal/runtime/observability"
	issuemodel "github.com/zeefan1555/symphony-go/internal/service/issue"
)

type retryReason string

const (
	retryContinuation retryReason = "continuation"
	retryFailure      retryReason = "failure"
)

func retryDelay(opts Options, reason retryReason, attempt int) time.Duration {
	if reason == retryContinuation {
		return time.Second
	}
	if attempt < 1 {
		attempt = 1
	}
	delay := 10 * time.Second
	for i := 1; i < attempt; i++ {
		delay *= 2
	}
	maxBackoff := maxRetryBackoff(opts)
	if delay > maxBackoff {
		return maxBackoff
	}
	return delay
}

func maxRetryBackoff(opts Options) time.Duration {
	if opts.Workflow != nil && opts.Workflow.Config.Agent.MaxRetryBackoffMS > 0 {
		return time.Duration(opts.Workflow.Config.Agent.MaxRetryBackoffMS) * time.Millisecond
	}
	return 300 * time.Second
}

func (o *Orchestrator) scheduleRetry(issue issuemodel.Issue, attempt int, reason retryReason, err error) {
	delay := retryDelay(o.currentOptions(), reason, attempt)
	dueAt := time.Now().Add(delay)

	o.mu.Lock()
	if old := o.retryTimers[issue.ID]; old != nil {
		old.Stop()
	}
	o.retryAttempts[issue.ID] = attempt
	entry := observability.RetryEntry{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		Attempt:         attempt,
		DueAt:           dueAt,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	replaced := false
	for i, existing := range o.snapshot.Retrying {
		if existing.IssueID == issue.ID {
			o.snapshot.Retrying[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		o.snapshot.Retrying = append(o.snapshot.Retrying, entry)
	}
	o.retryTimers[issue.ID] = o.newTimer(delay, func() {
		o.handleRetry(issue.ID)
	})
	o.mu.Unlock()
}

func (o *Orchestrator) clearRetry(issueID string) {
	o.mu.Lock()
	if old := o.retryTimers[issueID]; old != nil {
		old.Stop()
	}
	delete(o.retryTimers, issueID)
	delete(o.retryAttempts, issueID)
	o.removeRetryLocked(issueID)
	o.mu.Unlock()
}

func (o *Orchestrator) stopRetryTimers() {
	o.mu.Lock()
	defer o.mu.Unlock()
	for issueID, timer := range o.retryTimers {
		if timer != nil {
			timer.Stop()
		}
		delete(o.retryTimers, issueID)
	}
}
