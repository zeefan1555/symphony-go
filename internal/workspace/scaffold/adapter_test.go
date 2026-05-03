package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	generated "github.com/zeefan1555/symphony-go/internal/generated/hertz/scaffold/workspace"
	"github.com/zeefan1555/symphony-go/internal/types"
	coreworkspace "github.com/zeefan1555/symphony-go/internal/workspace"
)

func TestAdapterImplementsGeneratedWorkspaceScaffold(t *testing.T) {
	var _ generated.WorkspaceScaffold = (*Adapter)(nil)
}

func TestPrepareWorkspaceDelegatesToManagerAndPreservesContainment(t *testing.T) {
	root := t.TempDir()
	manager := coreworkspace.New(root, zeroHooks())
	adapter := NewAdapter(manager)

	preparation, err := adapter.PrepareWorkspace(context.Background(), &generated.WorkspacePrepareRequest{
		IssueIdentifier: "../ZEE/unsafe",
		RepoRoot:        root,
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

	_, err := adapter.CleanupWorkspace(context.Background(), &generated.WorkspaceCleanupRequest{
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

	projection, err := adapter.ResolveWorkspacePath(context.Background(), &generated.WorkspacePrepareRequest{
		IssueIdentifier: "ZEE-57",
		RepoRoot:        root,
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

func zeroHooks() types.HooksConfig {
	return types.HooksConfig{}
}
