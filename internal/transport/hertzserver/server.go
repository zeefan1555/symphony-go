package hertzserver

import (
	"context"
	"net"
	"sync"

	"github.com/cloudwego/hertz/pkg/app/server"
	"symphony-go/gen/hertz/router"
	controlplane "symphony-go/internal/service/control"
	"symphony-go/internal/transport/hertzbinding"
)

type Control = controlplane.ControlService

type Server struct {
	control Control

	mu      sync.Mutex
	hertz   *server.Hertz
	restore func()
}

func New(control Control) *Server {
	if control == nil {
		control = controlplane.NewService(nil)
	}
	return &Server{control: control}
}

func (s *Server) Serve(listener net.Listener) error {
	h := server.New(server.WithListener(listener))
	restore := hertzbinding.SetControlService(hertzbinding.NewControlBinding(s.control))
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
