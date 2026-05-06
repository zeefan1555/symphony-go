package codex

import (
	"context"
	"encoding/json"
	"errors"
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

func TestDynamicToolExecutorFailsWhenLinearAuthIsMissing(t *testing.T) {
	result := NewDynamicToolExecutor(nil).Execute(context.Background(), "linear_graphql", map[string]any{
		"query": "query Viewer { viewer { id } }",
	})
	if result.Success {
		t.Fatalf("success = true, want false")
	}
	for _, want := range []string{"missing Linear auth", "LINEAR_API_KEY"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output missing %q: %s", want, result.Output)
		}
	}
}

func TestDynamicToolExecutorReturnsTransportFailurePayload(t *testing.T) {
	client := &fakeGraphQLRawClient{err: errors.New("network timeout")}
	result := NewDynamicToolExecutor(client).Execute(context.Background(), "linear_graphql", map[string]any{
		"query": "query Viewer { viewer { id } }",
	})
	if result.Success {
		t.Fatalf("success = true, want false")
	}
	for _, want := range []string{"Linear GraphQL tool execution failed", "network timeout"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output missing %q: %s", want, result.Output)
		}
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

func TestDynamicToolExecutorFailsUnsupportedToolNames(t *testing.T) {
	result := NewDynamicToolExecutor(&fakeGraphQLRawClient{}).Execute(context.Background(), "unknown_tool", map[string]any{
		"query": "query Viewer { viewer { id } }",
	})
	if result.Success {
		t.Fatalf("success = true, want false")
	}
	for _, want := range []string{"Unsupported dynamic tool", "linear_graphql"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output missing %q: %s", want, result.Output)
		}
	}
}

func TestDynamicToolExecutorRequiresExactlyOneLinearGraphQLOperation(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "empty document with only fragment", query: "fragment UserFields on User { id }"},
		{name: "multiple named queries", query: "query One { viewer { id } } query Two { viewer { name } }"},
		{name: "anonymous plus named query", query: "{ viewer { id } } query Two { viewer { name } }"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := NewDynamicToolExecutor(&fakeGraphQLRawClient{}).Execute(context.Background(), "linear_graphql", tc.query)
			if result.Success {
				t.Fatalf("success = true, want false")
			}
			if !strings.Contains(result.Output, "`linear_graphql.query` must contain exactly one GraphQL operation") {
				t.Fatalf("output = %s", result.Output)
			}
		})
	}
}

func TestDynamicToolExecutorAcceptsOneOperationWithFragmentsAndStrings(t *testing.T) {
	client := &fakeGraphQLRawClient{}
	result := NewDynamicToolExecutor(client).Execute(context.Background(), "linear_graphql", `
query Viewer {
  viewer {
    ...UserFields
    note(text: "query Two { ignored }")
  }
}
fragment UserFields on User {
  id
  bio(text: """mutation Ignored { noop }""")
}
`)
	if !result.Success {
		t.Fatalf("success = false, output:\n%s", result.Output)
	}
	if !strings.Contains(client.query, "fragment UserFields") {
		t.Fatalf("query was not passed through: %q", client.query)
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
