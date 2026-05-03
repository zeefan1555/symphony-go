package http

import (
	"context"
	"sync"
)

type ScaffoldStatus struct {
	Status string
}

type ControlService interface {
	GetScaffold(context.Context) (ScaffoldStatus, error)
}

type ControlFunc func(context.Context) (ScaffoldStatus, error)

func (f ControlFunc) GetScaffold(ctx context.Context) (ScaffoldStatus, error) {
	return f(ctx)
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
