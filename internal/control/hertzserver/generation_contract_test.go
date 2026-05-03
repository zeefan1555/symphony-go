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
		"idl/control/http.thrift",
		"internal/generated/hertz/control",
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
		"`idl/control/common.thrift`",
		"`idl/control/http.thrift`",
		"`internal/generated/hertz/control/`",
		"`internal/control/hertzserver/`",
		"`internal/control/service.go`",
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
	commonIDL, err := os.ReadFile("../../../idl/control/common.thrift")
	if err != nil {
		t.Fatalf("read common IDL: %v", err)
	}
	if strings.Contains(string(commonIDL), "api.") {
		t.Fatalf("shared control model IDL must not contain Hertz api annotations")
	}

	httpIDL, err := os.ReadFile("../../../idl/control/http.thrift")
	if err != nil {
		t.Fatalf("read HTTP control IDL: %v", err)
	}
	if !strings.Contains(string(httpIDL), `api.get="/api/v1/scaffold"`) {
		t.Fatalf("HTTP control IDL must define the scaffold route annotation")
	}
}

func TestHertzScaffoldDoesNotOwnOrchestratorState(t *testing.T) {
	forbiddenImport := "internal/" + "orchestrator"
	for _, root := range []string{
		"../../../internal/generated/hertz/control",
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
	oldGeneratedPath := "internal/control/" + "hertzgen"
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
