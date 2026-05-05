package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	generated "github.com/zeefan1555/symphony-go/biz/model/workspace"
	runtimeconfig "github.com/zeefan1555/symphony-go/internal/runtime/config"
	coreworkspace "github.com/zeefan1555/symphony-go/internal/workspace"
)

func TestAdapterExposesStandardWorkspaceDiagnosticMethods(t *testing.T) {
	var _ interface {
		ResolveWorkspacePath(context.Context, *generated.ResolveWorkspacePathReq) (*generated.WorkspacePreparation, error)
		ValidateWorkspacePath(context.Context, *generated.ValidateWorkspacePathReq) (*generated.WorkspacePathValidation, error)
		PrepareWorkspace(context.Context, *generated.PrepareWorkspaceReq) (*generated.WorkspacePreparation, error)
		CleanupWorkspace(context.Context, *generated.CleanupWorkspaceReq) (*generated.WorkspaceCleanupResult, error)
	} = (*Adapter)(nil)
}

func TestPrepareWorkspaceDelegatesToManagerAndPreservesContainment(t *testing.T) {
	root := t.TempDir()
	manager := coreworkspace.New(root, zeroHooks())
	adapter := NewAdapter(manager)

	preparation, err := adapter.PrepareWorkspace(context.Background(), &generated.PrepareWorkspaceReq{
		IssueIdentifier: "../ZEE/unsafe",
	})
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	wantPath := filepath.Join(root, coreworkspace.SafeIdentifier("../ZEE/unsafe"))
	if preparation.WorkspacePath != wantPath {
		t.Fatalf("workspace path = %q, want %q", preparation.WorkspacePath, wantPath)
	}
	if !preparation.ContainedInRoot {
		t.Fatal("workspace path should be contained in root")
	}
	if info, err := os.Stat(preparation.WorkspacePath); err != nil || !info.IsDir() {
		t.Fatalf("prepared workspace should be a directory, info=%v err=%v", info, err)
	}
}

func TestCleanupWorkspaceRejectsEscapedPath(t *testing.T) {
	root := t.TempDir()
	manager := coreworkspace.New(root, zeroHooks())
	adapter := NewAdapter(manager)

	_, err := adapter.CleanupWorkspace(context.Background(), &generated.CleanupWorkspaceReq{
		WorkspacePath: filepath.Join(filepath.Dir(root), "outside"),
	})
	if err == nil {
		t.Fatal("CleanupWorkspace() error = nil, want escaped path rejection")
	}
}

func TestResolveWorkspacePathDoesNotCreateDirectory(t *testing.T) {
	root := t.TempDir()
	manager := coreworkspace.New(root, zeroHooks())
	adapter := NewAdapter(manager)

	projection, err := adapter.ResolveWorkspacePath(context.Background(), &generated.ResolveWorkspacePathReq{
		IssueIdentifier: "ZEE-57",
	})
	if err != nil {
		t.Fatalf("ResolveWorkspacePath() error = %v", err)
	}
	if !projection.ContainedInRoot {
		t.Fatal("resolved workspace path should be contained in root")
	}
	if _, err := os.Stat(projection.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("ResolveWorkspacePath should not create directory, stat err=%v", err)
	}
}

func zeroHooks() runtimeconfig.HooksConfig {
	return runtimeconfig.HooksConfig{}
}
