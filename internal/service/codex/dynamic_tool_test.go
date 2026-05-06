package codex

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDynamicToolExecutorAdvertisesLinearGraphQL(t *testing.T) {
	specs := NewDynamicToolExecutor(&fakeGraphQLRawClient{}).ToolSpecs()
	if len(specs) != 1 {
		t.Fatalf("tool specs = %#v, want one spec", specs)
	}
	spec, _ := specs[0].(map[string]any)
	if spec["name"] != "linear_graphql" {
		t.Fatalf("tool name = %#v, want linear_graphql", spec["name"])
	}
	schema, _ := spec["inputSchema"].(map[string]any)
	required, _ := schema["required"].([]any)
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("required = %#v, want query", required)
	}
}

func TestDynamicToolExecutorRunsLinearGraphQL(t *testing.T) {
	client := &fakeGraphQLRawClient{
		response: map[string]any{"data": map[string]any{"viewer": map[string]any{"id": "usr_123"}}},
	}
	result := NewDynamicToolExecutor(client).Execute(context.Background(), "linear_graphql", map[string]any{
		"query":     "  query Viewer { viewer { id } }  ",
		"variables": map[string]any{"includeTeams": false},
	})

	if !result.Success {
		t.Fatalf("success = false, output:\n%s", result.Output)
	}
	if client.query != "query Viewer { viewer { id } }" {
		t.Fatalf("query = %q", client.query)
	}
	if client.variables["includeTeams"] != false {
		t.Fatalf("variables = %#v", client.variables)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Output), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["data"]; !ok {
		t.Fatalf("output missing data: %#v", body)
	}
}

func TestDynamicToolExecutorPreservesGraphQLErrorsAsFailureOutput(t *testing.T) {
	client := &fakeGraphQLRawClient{
		response: map[string]any{"errors": []any{map[string]any{"message": "Unknown field"}}, "data": nil},
	}
	result := NewDynamicToolExecutor(client).Execute(context.Background(), "linear_graphql", "query Bad { nope }")

	if result.Success {
		t.Fatalf("success = true, want false")
	}
	if !strings.Contains(result.Output, "Unknown field") {
		t.Fatalf("output missing GraphQL error:\n%s", result.Output)
	}
}

func TestDynamicToolExecutorRejectsInvalidLinearGraphQLArguments(t *testing.T) {
	result := NewDynamicToolExecutor(&fakeGraphQLRawClient{}).Execute(context.Background(), "linear_graphql", map[string]any{
		"query":     "query Viewer { viewer { id } }",
		"variables": []any{"bad"},
	})
	if result.Success {
		t.Fatalf("success = true, want false")
	}
	if !strings.Contains(result.Output, "`linear_graphql.variables` must be a JSON object") {
		t.Fatalf("output = %s", result.Output)
	}
}

type fakeGraphQLRawClient struct {
	query     string
	variables map[string]any
	response  map[string]any
	err       error
}

func (c *fakeGraphQLRawClient) GraphQLRaw(_ context.Context, query string, variables map[string]any) (map[string]any, error) {
	c.query = query
	c.variables = variables
	if c.err != nil {
		return nil, c.err
	}
	if c.response != nil {
		return c.response, nil
	}
	return map[string]any{"data": map[string]any{}}, nil
}
