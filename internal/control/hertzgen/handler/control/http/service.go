package http

import (
	"context"
	"sync"

	model "github.com/zeefan1555/symphony-go/internal/control/hertzgen/model/control/model"
)

type ScaffoldStatus struct {
	Status string
}

type ControlService interface {
	GetScaffold(context.Context) (ScaffoldStatus, error)
	GetState(context.Context) (*model.RuntimeState, error)
}

type ControlFunc func(context.Context) (ScaffoldStatus, error)

func (f ControlFunc) GetScaffold(ctx context.Context) (ScaffoldStatus, error) {
	return f(ctx)
}

func (f ControlFunc) GetState(context.Context) (*model.RuntimeState, error) {
	return emptyRuntimeState(), nil
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

func getControlService() ControlService {
	controlService.RLock()
	defer controlService.RUnlock()
	return controlService.current
}

func emptyRuntimeState() *model.RuntimeState {
	return &model.RuntimeState{
		Counts:      &model.RuntimeCounts{},
		Running:     []*model.IssueRun{},
		Retrying:    []*model.RetryRun{},
		CodexTotals: &model.CodexTotals{},
		Polling:     &model.PollingStatus{},
	}
}
