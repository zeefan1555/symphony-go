package hertzserver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryDocumentsHertzGenerationCommand(t *testing.T) {
	makefile, err := os.ReadFile("../../../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	text := string(makefile)

	if !strings.Contains(text, ".PHONY:") || !strings.Contains(text, "hertz-generate") {
		t.Fatalf("Makefile must expose a hertz-generate target")
	}
	if !strings.Contains(text, "scripts/hertz_generate.sh") {
		t.Fatalf("hertz-generate target must call the documented generation script")
	}

	script, err := os.ReadFile("../../../scripts/hertz_generate.sh")
	if err != nil {
		t.Fatalf("read hertz generation script: %v", err)
	}
	scriptText := string(script)
	for _, want := range []string{
		"hz new",
		"idl/main.thrift",
		"biz/handler",
		"biz/model",
		"biz/router",
	} {
		if !strings.Contains(scriptText, want) {
			t.Fatalf("hertz generation script missing %q", want)
		}
	}
}

func TestMaintainerWorkflowDocumentsIDLBoundariesAndGeneration(t *testing.T) {
	doc, err := os.ReadFile("../../../docs/control-plane-hertz-idl.md")
	if err != nil {
		t.Fatalf("read maintainer workflow doc: %v", err)
	}
	text := string(doc)

	for _, want := range []string{
		"`idl/main.thrift`",
		"`idl/common.thrift`",
		"`idl/control.thrift`",
		"`biz/handler`",
		"`biz/model`",
		"`biz/router`",
		"`internal/control/hertzserver/`",
		"`internal/service/control/`",
		"`make hertz-generate`",
		"`scripts/hertz_generate.sh`",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("maintainer workflow doc missing %q", want)
		}
	}
}

func TestMaintainerWorkflowDocumentsReviewAndTransportBoundaries(t *testing.T) {
	doc, err := os.ReadFile("../../../docs/control-plane-hertz-idl.md")
	if err != nil {
		t.Fatalf("read maintainer workflow doc: %v", err)
	}
	text := string(doc)

	for _, want := range []string{
		"优先 review IDL 契约和手写 adapter",
		"公共模型 IDL 不能依赖 Hertz route annotations",
		"未来 Kitex 只能新增专用 adapter/IDL 层",
		"第一版不实现 Kitex runtime",
		"不把 `run --once --issue` 变成产品 API",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("maintainer workflow doc missing %q", want)
		}
	}
}

func TestIDLSeparatesSharedModelsFromHertzRoutes(t *testing.T) {
	commonIDL, err := os.ReadFile("../../../idl/common.thrift")
	if err != nil {
		t.Fatalf("read common IDL: %v", err)
	}
	if strings.Contains(string(commonIDL), "api.") {
		t.Fatalf("shared control model IDL must not contain Hertz api annotations")
	}

	mainIDL, err := os.ReadFile("../../../idl/main.thrift")
	if err != nil {
		t.Fatalf("read main IDL: %v", err)
	}
	if !strings.Contains(string(mainIDL), `api.post="/api/v1/control/get-scaffold"`) {
		t.Fatalf("main IDL must define the scaffold POST route annotation")
	}
	if strings.Contains(string(mainIDL), "api.get") {
		t.Fatalf("business routes in main IDL must use POST annotations")
	}
}

func TestUnifiedMainIDLControlsTopLevelContracts(t *testing.T) {
	mainIDL, err := os.ReadFile("../../../idl/main.thrift")
	if err != nil {
		t.Fatalf("read main IDL: %v", err)
	}
	mainText := string(mainIDL)
	if !strings.Contains(mainText, "service SymphonyAPI") {
		t.Fatalf("main IDL must own the single Hertz service")
	}
	for _, want := range []string{
		"control.GetScaffoldResp GetScaffold(1: control.GetScaffoldReq req)",
		"control.GetStateResp GetState(1: control.GetStateReq req)",
		"control.RefreshResp Refresh(1: control.RefreshReq req)",
		"control.GetIssueResp GetIssue(1: control.GetIssueReq req)",
	} {
		if !strings.Contains(mainText, want) {
			t.Fatalf("main IDL missing dedicated top-level contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"common.Empty",
		"common.RuntimeState GetState",
		"common.RefreshResult Refresh",
		"common.IssueDetail GetIssue",
	} {
		if strings.Contains(mainText, forbidden) {
			t.Fatalf("main IDL must not use shared model as top-level request/response: %q", forbidden)
		}
	}

	controlIDL, err := os.ReadFile("../../../idl/control.thrift")
	if err != nil {
		t.Fatalf("read control IDL: %v", err)
	}
	controlText := string(controlIDL)
	if strings.Contains(controlText, "service ") {
		t.Fatalf("control child IDL must not declare a service")
	}
	for _, want := range []string{
		"struct GetScaffoldReq",
		"struct GetScaffoldResp",
		"struct GetStateReq",
		"struct GetStateResp",
		"struct RefreshReq",
		"struct RefreshResp",
		"struct GetIssueReq",
		"struct GetIssueResp",
	} {
		if !strings.Contains(controlText, want) {
			t.Fatalf("control child IDL missing %q", want)
		}
	}
}

func TestHertzScaffoldDoesNotOwnOrchestratorState(t *testing.T) {
	forbiddenImport := "internal/" + "orchestrator"
	for _, root := range []string{
		"../../../biz",
		"../../../internal/control/hertzserver",
	} {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(content), forbiddenImport) {
				t.Fatalf("%s imports orchestrator internals; Hertz scaffold must stay an adapter", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

func TestProductionCodeDoesNotImportOldHertzgenPath(t *testing.T) {
	oldGeneratedPath := "internal/generated/hertz/" + "control"
	for _, root := range []string{
		"../../../cmd",
		"../../../internal",
		"../../../scripts",
	} {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(content), oldGeneratedPath) {
				t.Fatalf("%s imports old generated Hertz path", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}
