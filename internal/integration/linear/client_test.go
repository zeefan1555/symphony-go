package linear

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	runtimeconfig "symphony-go/internal/runtime/config"
)

func TestGraphQLSendsUTF8JSONHeadersAndBody(t *testing.T) {
	var sawChinese bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "lin_test" {
			t.Fatalf("Authorization = %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		variables := payload["variables"].(map[string]any)
		if variables["body"] == "zeefan 中文 smoke test" {
			sawChinese = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_test", HTTPClient: server.Client()}
	var out map[string]any
	if err := client.GraphQL(context.Background(), "mutation Test($body: String!) { ok }", map[string]any{"body": "zeefan 中文 smoke test"}, &out); err != nil {
		t.Fatal(err)
	}
	if !sawChinese {
		t.Fatal("server did not receive Chinese body")
	}
}

func TestRawGraphQLSendsUTF8JSONHeadersAuthAndEmptyVariables(t *testing.T) {
	var sawRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		if got := r.Header.Get("Authorization"); got != "lin_secret_token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Fatalf("Content-Type = %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["query"] != "query Viewer { viewer { id } }" {
			t.Fatalf("query = %#v", payload["query"])
		}
		variables, ok := payload["variables"].(map[string]any)
		if !ok || len(variables) != 0 {
			t.Fatalf("variables = %#v, want empty object", payload["variables"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"viewer":{"id":"usr_123"}}}`))
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_secret_token", HTTPClient: server.Client()}
	body, err := client.RawGraphQL(context.Background(), "query Viewer { viewer { id } }", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !sawRequest {
		t.Fatal("server did not receive request")
	}
	data := body["data"].(map[string]any)
	viewer := data["viewer"].(map[string]any)
	if viewer["id"] != "usr_123" {
		t.Fatalf("body = %#v", body)
	}
}

func TestRawGraphQLPreservesTopLevelErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"Unknown field"}]}`))
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_secret_token", HTTPClient: server.Client()}
	body, err := client.RawGraphQL(context.Background(), "query Bad { nope }", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	errorsValue, ok := body["errors"].([]any)
	if !ok || len(errorsValue) != 1 {
		t.Fatalf("errors = %#v", body["errors"])
	}
}

func TestRawGraphQLHTTPErrorDoesNotLeakToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary outage", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_secret_token", HTTPClient: server.Client()}
	_, err := client.RawGraphQL(context.Background(), "query Viewer { viewer { id } }", map[string]any{})
	if err == nil {
		t.Fatal("RawGraphQL error = nil, want HTTP status error")
	}
	text := err.Error()
	if !strings.Contains(text, "503") {
		t.Fatalf("error = %q, want status", text)
	}
	if strings.Contains(text, "lin_secret_token") {
		t.Fatalf("error leaked token: %q", text)
	}
}

func TestUpsertWorkpadUpdatesExistingChineseComment(t *testing.T) {
	var updated bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		query := payload["query"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(query, "comments"):
			_, _ = w.Write([]byte(`{"data":{"issue":{"comments":{"nodes":[{"id":"comment-1","body":"## Codex Workpad\n旧内容"}]}}}}`))
		case strings.Contains(query, "commentUpdate"):
			variables := payload["variables"].(map[string]any)
			if variables["body"] != "## Codex Workpad\n中文更新" {
				t.Fatalf("updated body = %#v", variables["body"])
			}
			updated = true
			_, _ = w.Write([]byte(`{"data":{"commentUpdate":{"success":true}}}`))
		default:
			t.Fatalf("unexpected query: %s", query)
		}
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_test", HTTPClient: server.Client()}
	if err := client.UpsertWorkpad(context.Background(), "issue-1", "## Codex Workpad\n中文更新"); err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("commentUpdate was not called")
	}
}

func TestFetchActiveIssuesPaginatesAndNormalizes(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"i1","identifier":"ZEE-1","title":"First","description":"Body","priority":1,"state":{"name":"Todo"},"branchName":"branch","url":"u","labels":{"nodes":[{"name":"AI"}]},"relations":{"nodes":[{"type":"blocks","relatedIssue":{"id":"out1","identifier":"ZEE-OUT","state":{"name":"Done"}}}]},"inverseRelations":{"nodes":[{"type":"blocks","issue":{"id":"b1","identifier":"ZEE-0","state":{"name":"In Progress"}}}]},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}],"pageInfo":{"hasNextPage":true,"endCursor":"cursor-1"}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"i2","identifier":"ZEE-2","title":"Second","description":"","priority":null,"state":{"name":"In Progress"},"branchName":"","url":"","labels":{"nodes":[{"name":"Bug"}]},"relations":{"nodes":[]},"inverseRelations":{"nodes":[]},"createdAt":"2026-01-03T00:00:00Z","updatedAt":"2026-01-04T00:00:00Z"}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`))
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_test", ProjectSlug: "demo", HTTPClient: server.Client()}
	issues, err := client.FetchActiveIssues(context.Background(), []string{"Todo", "In Progress"})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
	if issues[0].Labels[0] != "ai" {
		t.Fatalf("labels = %#v", issues[0].Labels)
	}
	if len(issues[0].BlockedBy) != 1 || issues[0].BlockedBy[0].Identifier != "ZEE-0" {
		t.Fatalf("blockers = %#v", issues[0].BlockedBy)
	}
	if issues[0].CreatedAt == nil || issues[1].UpdatedAt == nil {
		t.Fatalf("timestamps were not parsed: %#v %#v", issues[0], issues[1])
	}
}

func TestNewAppliesLinearDefaultsAndEnvAPIKey(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_env")

	client, err := New(runtimeconfig.TrackerConfig{
		Kind:        "linear",
		ProjectSlug: "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.Endpoint != "https://api.linear.app/graphql" {
		t.Fatalf("endpoint = %q", client.Endpoint)
	}
	if client.APIKey != "lin_env" {
		t.Fatalf("api key = %q", client.APIKey)
	}
	if client.ProjectSlug != "demo" {
		t.Fatalf("project slug = %q", client.ProjectSlug)
	}
	if client.HTTPClient == nil || client.HTTPClient.Timeout != 30*time.Second {
		t.Fatalf("timeout = %#v, want 30s", client.HTTPClient)
	}
}

func TestFetchIssueStatesByIDsEmptyDoesNotCallAPI(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_test", ProjectSlug: "demo", HTTPClient: server.Client()}
	issues, err := client.FetchIssueStatesByIDs(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if called {
		t.Fatal("empty state refresh should not call Linear")
	}
}

func TestFetchIssuesByStatesEmptyDoesNotCallAPI(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_test", ProjectSlug: "demo", HTTPClient: server.Client()}
	issues, err := client.FetchIssuesByStates(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if called {
		t.Fatal("empty state fetch should not call Linear")
	}
}

func TestFetchIssuesByStatesUsesProvidedStatesAndNormalizes(t *testing.T) {
	var sawStates bool
	var sawInverseRelations bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		query := payload["query"].(string)
		if strings.Contains(query, "inverseRelations") && strings.Contains(query, "issue {") {
			sawInverseRelations = true
		}
		variables := payload["variables"].(map[string]any)
		states := variables["stateNames"].([]any)
		if len(states) == 1 && states[0] == "Done" {
			sawStates = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"i1","identifier":"ZEE-1","title":"Terminal","description":"","priority":null,"state":{"name":"Done"},"branchName":"","url":"","labels":{"nodes":[{"name":"AI"}]},"relations":{"nodes":[{"type":"blocks","relatedIssue":{"id":"out1","identifier":"ZEE-OUT","state":{"name":"Done"}}}]},"inverseRelations":{"nodes":[{"type":"blocks","issue":{"id":"b1","identifier":"ZEE-0","state":{"name":"In Progress"}}}]},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`))
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_test", ProjectSlug: "demo", HTTPClient: server.Client()}
	issues, err := client.FetchIssuesByStates(context.Background(), []string{"Done"})
	if err != nil {
		t.Fatal(err)
	}
	if !sawStates {
		t.Fatal("FetchIssuesByStates did not send provided states")
	}
	if !sawInverseRelations {
		t.Fatal("FetchIssuesByStates did not query inverseRelations issue blockers")
	}
	if len(issues) != 1 || issues[0].State != "Done" || issues[0].Labels[0] != "ai" {
		t.Fatalf("issues = %#v", issues)
	}
	if len(issues[0].BlockedBy) != 1 || issues[0].BlockedBy[0].Identifier != "ZEE-0" {
		t.Fatalf("blockers = %#v", issues[0].BlockedBy)
	}
}

func TestFetchIssueStatesByIDsUsesIDListQueryAndNormalizes(t *testing.T) {
	var sawIDs bool
	var sawIDType bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		query := payload["query"].(string)
		if strings.Contains(query, "$ids: [ID!]") {
			sawIDType = true
		}
		variables := payload["variables"].(map[string]any)
		ids := variables["ids"].([]any)
		if len(ids) == 2 && ids[0] == "i1" && ids[1] == "i2" {
			sawIDs = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"i1","identifier":"ZEE-1","state":{"name":"Done"}},{"id":"i2","identifier":"ZEE-2","state":{"name":"Todo"}}]}}}`))
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_test", ProjectSlug: "demo", HTTPClient: server.Client()}
	issues, err := client.FetchIssueStatesByIDs(context.Background(), []string{"i1", "i2"})
	if err != nil {
		t.Fatal(err)
	}
	if !sawIDType {
		t.Fatal("FetchIssueStatesByIDs query did not use [ID!] variable type")
	}
	if !sawIDs {
		t.Fatal("FetchIssueStatesByIDs did not send ids variable")
	}
	if len(issues) != 2 || issues[0].Identifier != "ZEE-1" || issues[1].State != "Todo" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestFetchIssuesByStatesMissingEndCursorReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"i1","identifier":"ZEE-1","title":"First","description":"","priority":null,"state":{"name":"Todo"},"branchName":"","url":"","labels":{"nodes":[]},"relations":{"nodes":[]},"createdAt":"","updatedAt":""}],"pageInfo":{"hasNextPage":true,"endCursor":null}}}}`))
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_test", ProjectSlug: "demo", HTTPClient: server.Client()}
	issues, err := client.FetchIssuesByStates(context.Background(), []string{"Todo"})
	if err == nil {
		t.Fatalf("expected error, got issues = %#v", issues)
	}
	if !strings.Contains(err.Error(), "linear_missing_end_cursor") {
		t.Fatalf("err = %v", err)
	}
}
