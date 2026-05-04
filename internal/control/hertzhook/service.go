package hertzhook

import (
	"context"
	"errors"
	"sync"

	commonmodel "github.com/zeefan1555/symphony-go/biz/model/common"
	controlmodel "github.com/zeefan1555/symphony-go/biz/model/control"
)

type ScaffoldStatus struct {
	Status string
}

type ControlService interface {
	GetScaffold(context.Context) (ScaffoldStatus, error)
	GetState(context.Context) (*commonmodel.RuntimeState, error)
	GetIssue(context.Context, string) (*commonmodel.IssueDetail, error)
	Refresh(context.Context) (*controlmodel.RefreshResp, error)
}

type ControlFunc func(context.Context) (ScaffoldStatus, error)

func (f ControlFunc) GetScaffold(ctx context.Context) (ScaffoldStatus, error) {
	return f(ctx)
}

func (f ControlFunc) GetState(context.Context) (*commonmodel.RuntimeState, error) {
	return emptyRuntimeState(), nil
}

func (f ControlFunc) GetIssue(context.Context, string) (*commonmodel.IssueDetail, error) {
	return nil, NewError(404, "issue_not_found", "issue not found")
}

func (f ControlFunc) Refresh(context.Context) (*controlmodel.RefreshResp, error) {
	return nil, NewError(503, "refresh_unavailable", "refresh trigger is unavailable")
}

var controlService = struct {
	sync.RWMutex
	current ControlService
}{
	current: ControlFunc(func(context.Context) (ScaffoldStatus, error) {
		return ScaffoldStatus{Status: "unconfigured"}, nil
	}),
}

func SetControlService(service ControlService) func() {
	if service == nil {
		service = ControlFunc(func(context.Context) (ScaffoldStatus, error) {
			return ScaffoldStatus{Status: "unconfigured"}, nil
		})
	}

	controlService.Lock()
	previous := controlService.current
	controlService.current = service
	controlService.Unlock()

	return func() {
		controlService.Lock()
		controlService.current = previous
		controlService.Unlock()
	}
}

func CurrentService() ControlService {
	controlService.RLock()
	defer controlService.RUnlock()
	return controlService.current
}

func emptyRuntimeState() *commonmodel.RuntimeState {
	return &commonmodel.RuntimeState{
		Counts:      &commonmodel.RuntimeCounts{},
		Running:     []*commonmodel.IssueRun{},
		Retrying:    []*commonmodel.RetryRun{},
		CodexTotals: &commonmodel.CodexTotals{},
		Polling:     &commonmodel.PollingStatus{},
	}
}

type Error struct {
	status  int
	code    string
	message string
}

func NewError(status int, code, message string) *Error {
	return &Error{status: status, code: code, message: message}
}

func (e *Error) Error() string {
	return e.message
}

func (e *Error) StatusCode() int {
	return e.status
}

func (e *Error) ErrorCode() string {
	return e.code
}

func (e *Error) Message() string {
	return e.message
}

func ErrorEnvelope(err error) (*commonmodel.ErrorEnvelope, int) {
	var controlErr *Error
	if errors.As(err, &controlErr) {
		return &commonmodel.ErrorEnvelope{Error: &commonmodel.ErrorDetail{
			Code:    controlErr.ErrorCode(),
			Message: controlErr.Message(),
		}}, controlErr.StatusCode()
	}
	return &commonmodel.ErrorEnvelope{Error: &commonmodel.ErrorDetail{
		Code:    "internal_error",
		Message: err.Error(),
	}}, 500
}
