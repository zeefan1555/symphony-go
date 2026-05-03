package hertzserver_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/zeefan1555/symphony-go/internal/control"
	"github.com/zeefan1555/symphony-go/internal/control/hertzserver"
	"github.com/zeefan1555/symphony-go/internal/observability"
)

type snapshotProvider struct {
	snapshot observability.Snapshot
}

func (p snapshotProvider) Snapshot() observability.Snapshot {
	return p.snapshot
}

func TestScaffoldRouteCallsAuthoredControlService(t *testing.T) {
	service := control.NewService(snapshotProvider{snapshot: observability.NewSnapshot()})
	server := hertzserver.New(service)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	})

	req, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+"/api/v1/scaffold", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		select {
		case serveErr := <-errCh:
			t.Fatalf("server exited early: %v", serveErr)
		default:
		}
		t.Fatalf("GET /api/v1/scaffold: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("response status = %q, want ok", body.Status)
	}
}
