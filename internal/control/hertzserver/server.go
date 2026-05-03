package hertzserver

import (
	"context"
	"net"
	"sync"

	"github.com/cloudwego/hertz/pkg/app/server"
	controlhttp "github.com/zeefan1555/symphony-go/internal/control/hertzgen/handler/control/http"
	"github.com/zeefan1555/symphony-go/internal/control/hertzgen/router"
)

type ScaffoldStatus struct {
	Status string
}

type Control interface {
	GetScaffold(context.Context) (ScaffoldStatus, error)
}

type ControlFunc func(context.Context) (ScaffoldStatus, error)

func (f ControlFunc) GetScaffold(ctx context.Context) (ScaffoldStatus, error) {
	return f(ctx)
}

type Server struct {
	control Control

	mu      sync.Mutex
	hertz   *server.Hertz
	restore func()
}

func New(control Control) *Server {
	if control == nil {
		control = ControlFunc(func(context.Context) (ScaffoldStatus, error) {
			return ScaffoldStatus{Status: "unconfigured"}, nil
		})
	}
	return &Server{control: control}
}

func (s *Server) Serve(listener net.Listener) error {
	h := server.New(server.WithListener(listener))
	restore := controlhttp.SetControlService(controlAdapter{control: s.control})
	router.GeneratedRegister(h)

	s.mu.Lock()
	s.hertz = h
	s.restore = restore
	s.mu.Unlock()

	return h.Run()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	h := s.hertz
	restore := s.restore
	s.hertz = nil
	s.restore = nil
	s.mu.Unlock()

	if restore != nil {
		restore()
	}
	if h == nil {
		return nil
	}
	return h.Shutdown(ctx)
}

type controlAdapter struct {
	control Control
}

func (a controlAdapter) GetScaffold(ctx context.Context) (controlhttp.ScaffoldStatus, error) {
	status, err := a.control.GetScaffold(ctx)
	if err != nil {
		return controlhttp.ScaffoldStatus{}, err
	}
	return controlhttp.ScaffoldStatus{Status: status.Status}, nil
}
