package hertzserver_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type hertzMethodContract struct {
	Domain string
	Method string
	Route  string
}

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
		"gen/hertz/handler",
		"gen/hertz/model",
		"gen/hertz/router",
	} {
		if !strings.Contains(scriptText, want) {
			t.Fatalf("hertz generation script missing %q", want)
		}
	}
	if strings.Contains(scriptText, "$repo_root/biz") {
		t.Fatalf("hertz generation script must not write generated Hertz output to biz")
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
		"`idl/orchestrator.thrift`",
		"`idl/workspace.thrift`",
		"`idl/workflow.thrift`",
		"`idl/codex_session.thrift`",
		"`gen/hertz/handler`",
		"`gen/hertz/model`",
		"`gen/hertz/router`",
		"`internal/transport/hertzbinding/`",
		"`internal/transport/hertzserver/`",
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
		"优先 review IDL 契约和手写传输层",
		"公共模型 IDL 不能依赖 Hertz route annotations",
		"未来 Kitex 只能新增专用 RPC 传输层和 IDL",
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
	mainText := string(mainIDL)
	if !strings.Contains(mainText, `api.post="/api/v1/control/get-scaffold"`) {
		t.Fatalf("main IDL must define the scaffold POST route annotation")
	}
	if strings.Contains(mainText, "api.get") {
		t.Fatalf("business routes in main IDL must use POST annotations")
	}

	for _, path := range childContractIDLPaths() {
		childIDL, err := os.ReadFile(filepath.Join("../../../idl", path))
		if err != nil {
			t.Fatalf("read child IDL %s: %v", path, err)
		}
		childText := string(childIDL)
		for _, forbidden := range []string{"api.get", "api.post", "api.path"} {
			if strings.Contains(childText, forbidden) {
				t.Fatalf("%s must not own business route annotation %q", path, forbidden)
			}
		}
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
	methods := parseHertzMethods(t, mainText)
	if len(methods) != len(expectedHertzMethodContracts()) {
		t.Fatalf("main IDL method count = %d, want %d", len(methods), len(expectedHertzMethodContracts()))
	}

	expected := map[string]hertzMethodContract{}
	for _, contract := range expectedHertzMethodContracts() {
		expected[contract.Method] = contract
	}
	seen := map[string]bool{}
	for _, method := range methods {
		contract, ok := expected[method.Method]
		if !ok {
			t.Fatalf("unexpected main IDL method: %#v", method)
		}
		seen[method.Method] = true
		if method.Domain != contract.Domain || method.Route != contract.Route {
			t.Fatalf("method %s = %#v, want %#v", method.Method, method, contract)
		}
		expectedSignature := method.Domain + "." + method.Method + "Resp " + method.Method + "(1: " + method.Domain + "." + method.Method + "Req req)"
		if !strings.Contains(mainText, expectedSignature) {
			t.Fatalf("main IDL method %s must use dedicated XxxReq/XxxResp signature %q", method.Method, expectedSignature)
		}
	}
	for method := range expected {
		if !seen[method] {
			t.Fatalf("main IDL missing method %s", method)
		}
	}
	if strings.Contains(mainText, "common.Empty") {
		t.Fatalf("main IDL must not use shared Empty top-level request/response")
	}
}

func TestChildIDLFilesOnlyDefineContracts(t *testing.T) {
	methods := expectedHertzMethodContracts()
	methodsByDomain := map[string][]string{}
	for _, method := range methods {
		methodsByDomain[method.Domain] = append(methodsByDomain[method.Domain], method.Method)
	}

	for domain, path := range childContractIDLPathByDomain() {
		content, err := os.ReadFile(filepath.Join("../../../idl", path))
		if err != nil {
			t.Fatalf("read child IDL %s: %v", path, err)
		}
		text := string(content)
		if strings.Contains(text, "service ") {
			t.Fatalf("%s child IDL must not declare a service", path)
		}
		for _, method := range methodsByDomain[domain] {
			for _, suffix := range []string{"Req", "Resp"} {
				want := "struct " + method + suffix
				if !strings.Contains(text, want) {
					t.Fatalf("%s child IDL missing %q", path, want)
				}
			}
		}
	}
}

func TestAllBusinessRoutesArePostActionRoutes(t *testing.T) {
	mainIDL, err := os.ReadFile("../../../idl/main.thrift")
	if err != nil {
		t.Fatalf("read main IDL: %v", err)
	}
	mainText := string(mainIDL)
	for _, forbidden := range []string{"api.get", "api.put", "api.delete", "api.patch"} {
		if strings.Contains(mainText, forbidden) {
			t.Fatalf("business routes in main IDL must not use %s", forbidden)
		}
	}
	for _, method := range parseHertzMethods(t, mainText) {
		if !strings.HasPrefix(method.Route, "/api/v1/") {
			t.Fatalf("route %s must live under /api/v1", method.Route)
		}
		if !strings.Contains(method.Route, "/") || strings.Contains(method.Route, "{") || strings.Contains(method.Route, ":") {
			t.Fatalf("route %s must be a concrete action route", method.Route)
		}
	}
}

func TestGeneratedRouterRegistersAllBusinessEndpoints(t *testing.T) {
	routerGo, err := os.ReadFile("../../../gen/hertz/router/api/main.go")
	if err != nil {
		t.Fatalf("read generated router: %v", err)
	}
	routerText := string(routerGo)
	for _, contract := range expectedHertzMethodContracts() {
		parts := strings.Split(contract.Route, "/")
		if len(parts) < 5 {
			t.Fatalf("route %s has unexpected shape", contract.Route)
		}
		group := parts[3]
		action := "/" + parts[4]
		if len(parts) > 5 {
			action = "/" + strings.Join(parts[4:], "/")
		}
		if !strings.Contains(routerText, `Group("/`+group+`"`) {
			t.Fatalf("generated router missing group for %s", contract.Route)
		}
		if !strings.Contains(routerText, `POST("`+action+`"`) || !strings.Contains(routerText, contract.Method) {
			t.Fatalf("generated router missing POST route for %s", contract.Route)
		}
	}
}

func TestHertzScaffoldDoesNotOwnOrchestratorState(t *testing.T) {
	forbiddenImports := []string{
		"internal/" + "orchestrator",
		"internal/" + "workspace",
		"internal/" + "codex",
		"internal/" + "workflow",
		"internal/" + "integration",
	}
	for _, root := range []string{
		"../../../gen/hertz",
		"../../../internal/transport/hertzserver",
	} {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, forbiddenImport := range forbiddenImports {
				if strings.Contains(string(content), forbiddenImport) {
					t.Fatalf("%s imports %s; Hertz scaffold must stay transport-only", path, forbiddenImport)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

func parseHertzMethods(t *testing.T, mainText string) []hertzMethodContract {
	t.Helper()
	methodPattern := regexp.MustCompile(`(?m)^\s+([a-z_]+)\.([A-Za-z0-9]+)Resp\s+([A-Za-z0-9]+)\(1:\s+([a-z_]+)\.([A-Za-z0-9]+)Req req\)\s+\(api\.post="([^"]+)"\)`)
	matches := methodPattern.FindAllStringSubmatch(mainText, -1)
	methods := make([]hertzMethodContract, 0, len(matches))
	for _, match := range matches {
		respDomain := match[1]
		respMethod := match[2]
		method := match[3]
		reqDomain := match[4]
		reqMethod := match[5]
		if respDomain != reqDomain {
			t.Fatalf("method %s uses mismatched domains: resp=%s req=%s", method, respDomain, reqDomain)
		}
		if respMethod != method || reqMethod != method {
			t.Fatalf("method %s must use dedicated XxxReq/XxxResp, got %sResp/%sReq", method, respMethod, reqMethod)
		}
		methods = append(methods, hertzMethodContract{Domain: respDomain, Method: method, Route: match[6]})
	}
	return methods
}

func expectedHertzMethodContracts() []hertzMethodContract {
	return []hertzMethodContract{
		{Domain: "control", Method: "GetScaffold", Route: "/api/v1/control/get-scaffold"},
		{Domain: "control", Method: "GetState", Route: "/api/v1/control/get-state"},
		{Domain: "control", Method: "Refresh", Route: "/api/v1/control/refresh"},
		{Domain: "control", Method: "GetIssue", Route: "/api/v1/control/get-issue"},
		{Domain: "orchestrator", Method: "ProjectIssueRun", Route: "/api/v1/orchestrator/project-issue-run"},
		{Domain: "workspace", Method: "ResolveWorkspacePath", Route: "/api/v1/workspace/resolve"},
		{Domain: "workspace", Method: "ValidateWorkspacePath", Route: "/api/v1/workspace/validate"},
		{Domain: "workspace", Method: "PrepareWorkspace", Route: "/api/v1/workspace/prepare"},
		{Domain: "workspace", Method: "CleanupWorkspace", Route: "/api/v1/workspace/cleanup"},
		{Domain: "workflow", Method: "LoadWorkflow", Route: "/api/v1/workflow/load"},
		{Domain: "workflow", Method: "RenderWorkflowPrompt", Route: "/api/v1/workflow/render-prompt"},
		{Domain: "codex_session", Method: "RunTurn", Route: "/api/v1/codex-session/run-turn"},
	}
}

func childContractIDLPaths() []string {
	paths := make([]string, 0, len(childContractIDLPathByDomain()))
	for _, path := range childContractIDLPathByDomain() {
		paths = append(paths, path)
	}
	return paths
}

func childContractIDLPathByDomain() map[string]string {
	return map[string]string{
		"control":       "control.thrift",
		"orchestrator":  "orchestrator.thrift",
		"workspace":     "workspace.thrift",
		"workflow":      "workflow.thrift",
		"codex_session": "codex_session.thrift",
	}
}

func TestProductionCodeDoesNotImportOldInternalGeneratedPath(t *testing.T) {
	oldGeneratedRoot := "internal/" + "generated"
	if _, err := os.Stat(filepath.Join("../../../", oldGeneratedRoot)); err == nil {
		t.Fatalf("old internal generated root still exists: %s", oldGeneratedRoot)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", oldGeneratedRoot, err)
	}

	for _, root := range []string{
		"../../../cmd",
		"../../../internal",
		"../../../scripts",
	} {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(content), oldGeneratedRoot) {
				t.Fatalf("%s imports old internal generated path", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}
