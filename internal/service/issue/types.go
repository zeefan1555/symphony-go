package issue

import "time"

type BlockerRef struct {
	ID         string
	Identifier string
	State      string
}

type Issue struct {
	ID          string
	Identifier  string
	Title       string
	Description string
	Priority    *int
	State       string
	BranchName  string
	URL         string
	AssigneeID  string
	// AssignedToWorker is nil when the tracker did not evaluate worker routing.
	AssignedToWorker *bool
	Labels           []string
	BlockedBy        []BlockerRef
	CreatedAt        *time.Time
	UpdatedAt        *time.Time
}
