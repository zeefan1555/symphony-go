package hertzserver

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	commonmodel "symphony-go/gen/hertz/model/common"
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
	h.GET("/", func(ctx context.Context, c *app.RequestContext) {
		state, err := binding.GetState(ctx)
		if err != nil {
			envelope, status := hertzbinding.ErrorEnvelope(err)
			c.JSON(status, envelope)
			return
		}
		c.Data(consts.StatusOK, "text/plain; charset=utf-8", []byte(renderDashboard(state)))
	})
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

func renderDashboard(state *commonmodel.RuntimeState) string {
	if state == nil {
		return "SYMPHONY STATUS\n\nRuntime state unavailable\n"
	}

	counts := state.GetCounts()
	totals := state.GetCodexTotals()
	polling := state.GetPolling()

	var b strings.Builder
	fmt.Fprintln(&b, "SYMPHONY STATUS")
	if generatedAt := state.GetGeneratedAt(); generatedAt != "" {
		fmt.Fprintf(&b, "Generated: %s\n", generatedAt)
	}
	fmt.Fprintf(&b, "Agents: %d running / %d retrying\n", counts.GetRunning(), counts.GetRetrying())
	fmt.Fprintf(&b, "Tokens: in %d | out %d | total %d\n",
		totals.GetInputTokens(),
		totals.GetOutputTokens(),
		totals.GetTotalTokens(),
	)
	fmt.Fprintf(&b, "Polling: interval=%dms checking=%t next_in=%dms\n",
		polling.GetIntervalMs(),
		polling.GetChecking(),
		polling.GetNextPollInMs(),
	)
	if lastError := state.GetLastError(); lastError != "" {
		fmt.Fprintf(&b, "Last error: %s\n", lastError)
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Running")
	if len(state.GetRunning()) == 0 {
		fmt.Fprintln(&b, "- none")
	} else {
		for _, run := range state.GetRunning() {
			tokens := run.GetTokens()
			fmt.Fprintf(&b, "- %s state=%s stage=%s pid=%d turns=%d tokens=%d session=%s event=%s\n",
				defaultString(run.GetIssueIdentifier(), run.GetIssueID()),
				defaultString(run.GetState(), "-"),
				dashboardStage(run),
				run.GetPid(),
				run.GetTurnCount(),
				tokens.GetTotalTokens(),
				defaultString(run.GetSessionID(), "-"),
				defaultString(run.GetLastEvent(), "-"),
			)
		}
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Retrying")
	if len(state.GetRetrying()) == 0 {
		fmt.Fprintln(&b, "- none")
	} else {
		for _, retry := range state.GetRetrying() {
			fmt.Fprintf(&b, "- %s attempt=%d due_at=%s error=%s\n",
				defaultString(retry.GetIssueIdentifier(), retry.GetIssueID()),
				retry.GetAttempt(),
				defaultString(retry.GetDueAt(), "-"),
				defaultString(retry.GetError(), "-"),
			)
		}
	}

	return b.String()
}

func dashboardStage(run *commonmodel.IssueRun) string {
	if run.GetAgentPhase() != "" && run.GetStage() != "" {
		return run.GetAgentPhase() + "/" + run.GetStage()
	}
	return defaultString(run.GetStage(), "-")
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
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
