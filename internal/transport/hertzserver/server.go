package hertzserver

import (
	"context"
	"net"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
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
	binding := hertzbinding.NewControlBinding(s.control)
	restore := hertzbinding.SetControlService(binding)
	router.GeneratedRegister(h)
	registerSpecAliases(h, binding)

	s.mu.Lock()
	s.hertz = h
	s.restore = restore
	s.mu.Unlock()

	return h.Run()
}

func registerSpecAliases(h *server.Hertz, binding hertzbinding.ControlService) {
	h.GET("/api/v1/state", func(ctx context.Context, c *app.RequestContext) {
		state, err := binding.GetState(ctx)
		if err != nil {
			envelope, status := hertzbinding.ErrorEnvelope(err)
			c.JSON(status, envelope)
			return
		}
		c.JSON(consts.StatusOK, map[string]any{"state": state})
	})
	h.GET("/api/v1/:issue_identifier", func(ctx context.Context, c *app.RequestContext) {
		issueIdentifier := c.Param("issue_identifier")
		detail, err := binding.GetIssue(ctx, issueIdentifier)
		if err != nil {
			envelope, status := hertzbinding.ErrorEnvelope(err)
			c.JSON(status, envelope)
			return
		}
		c.JSON(consts.StatusOK, map[string]any{"issue": detail})
	})
	h.POST("/api/v1/refresh", func(ctx context.Context, c *app.RequestContext) {
		resp, err := binding.Refresh(ctx)
		if err != nil {
			envelope, status := hertzbinding.ErrorEnvelope(err)
			c.JSON(status, envelope)
			return
		}
		c.JSON(consts.StatusAccepted, resp)
	})
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
