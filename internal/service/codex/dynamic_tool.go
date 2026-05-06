package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const linearGraphQLToolName = "linear_graphql"

type graphQLRawClient interface {
	GraphQLRaw(context.Context, string, map[string]any) (map[string]any, error)
}

type DynamicToolExecutor struct {
	linear graphQLRawClient
}

func NewDynamicToolExecutor(linear graphQLRawClient) *DynamicToolExecutor {
	return &DynamicToolExecutor{linear: linear}
}

func LinearGraphQLToolSpecs() []any {
	return []any{
		map[string]any{
			"name":        linearGraphQLToolName,
			"description": "Execute a raw GraphQL query or mutation against Linear using Symphony's configured auth.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"query"},
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "GraphQL query or mutation document to execute against Linear.",
					},
					"variables": map[string]any{
						"type":                 []any{"object", "null"},
						"description":          "Optional GraphQL variables object.",
						"additionalProperties": true,
					},
				},
			},
		},
	}
}

func (e *DynamicToolExecutor) ToolSpecs() []any {
	if e == nil || e.linear == nil {
		return []any{}
	}
	return LinearGraphQLToolSpecs()
}

func (e *DynamicToolExecutor) Execute(ctx context.Context, tool string, arguments any) dynamicToolResult {
	if tool != linearGraphQLToolName {
		return dynamicToolFailure(map[string]any{
			"error": map[string]any{
				"message":        fmt.Sprintf("Unsupported dynamic tool: %q.", tool),
				"supportedTools": []any{linearGraphQLToolName},
			},
		})
	}
	if e == nil || e.linear == nil {
		return dynamicToolFailure(map[string]any{
			"error": map[string]any{
				"message": "Symphony is missing Linear auth. Set tracker.api_key in WORKFLOW.md or export LINEAR_API_KEY.",
			},
		})
	}
	query, variables, err := normalizeLinearGraphQLArguments(arguments)
	if err != nil {
		return dynamicToolFailure(linearGraphQLErrorPayload(err))
	}
	response, err := e.linear.GraphQLRaw(ctx, query, variables)
	if err != nil {
		return dynamicToolFailure(map[string]any{
			"error": map[string]any{
				"message": "Linear GraphQL tool execution failed.",
				"reason":  err.Error(),
			},
		})
	}
	return dynamicToolResponse(!hasGraphQLErrors(response), response)
}

type dynamicToolResult struct {
	Success      bool             `json:"success"`
	Output       string           `json:"output"`
	ContentItems []map[string]any `json:"contentItems"`
}

func dynamicToolResponse(success bool, payload any) dynamicToolResult {
	output := encodeDynamicToolPayload(payload)
	return dynamicToolResult{
		Success: success,
		Output:  output,
		ContentItems: []map[string]any{
			{"type": "inputText", "text": output},
		},
	}
}

func dynamicToolFailure(payload any) dynamicToolResult {
	return dynamicToolResponse(false, payload)
}

type linearGraphQLArgumentError string

const (
	errMissingQuery     linearGraphQLArgumentError = "missing_query"
	errInvalidArguments linearGraphQLArgumentError = "invalid_arguments"
	errInvalidVariables linearGraphQLArgumentError = "invalid_variables"
	errInvalidOperation linearGraphQLArgumentError = "invalid_operation_count"
)

func (e linearGraphQLArgumentError) Error() string {
	return string(e)
}

func normalizeLinearGraphQLArguments(arguments any) (string, map[string]any, error) {
	switch value := arguments.(type) {
	case string:
		query := strings.TrimSpace(value)
		if query == "" {
			return "", nil, errMissingQuery
		}
		if !hasExactlyOneGraphQLOperation(query) {
			return "", nil, errInvalidOperation
		}
		return query, map[string]any{}, nil
	case map[string]any:
		rawQuery, _ := value["query"].(string)
		query := strings.TrimSpace(rawQuery)
		if query == "" {
			return "", nil, errMissingQuery
		}
		if !hasExactlyOneGraphQLOperation(query) {
			return "", nil, errInvalidOperation
		}
		rawVariables, ok := value["variables"]
		if !ok || rawVariables == nil {
			return query, map[string]any{}, nil
		}
		variables, ok := rawVariables.(map[string]any)
		if !ok {
			return "", nil, errInvalidVariables
		}
		return query, variables, nil
	default:
		return "", nil, errInvalidArguments
	}
}

func linearGraphQLErrorPayload(err error) map[string]any {
	message := "Linear GraphQL tool execution failed."
	switch err {
	case errMissingQuery:
		message = "`linear_graphql` requires a non-empty `query` string."
	case errInvalidArguments:
		message = "`linear_graphql` expects either a GraphQL query string or an object with `query` and optional `variables`."
	case errInvalidVariables:
		message = "`linear_graphql.variables` must be a JSON object when provided."
	case errInvalidOperation:
		message = "`linear_graphql.query` must contain exactly one GraphQL operation."
	}
	return map[string]any{"error": map[string]any{"message": message}}
}

func hasExactlyOneGraphQLOperation(query string) bool {
	depth := 0
	operations := 0
	pendingDefinitionBody := false
	for i := 0; i < len(query); {
		switch query[i] {
		case '#':
			i = skipGraphQLLineComment(query, i+1)
			continue
		case '"':
			var ok bool
			i, ok = skipGraphQLString(query, i)
			if !ok {
				return false
			}
			continue
		case '{':
			if depth == 0 {
				if pendingDefinitionBody {
					pendingDefinitionBody = false
				} else {
					operations++
					if operations > 1 {
						return false
					}
				}
			}
			depth++
			i++
			continue
		case '}':
			if depth > 0 {
				depth--
			}
			i++
			continue
		}

		if depth == 0 && isGraphQLNameStart(query[i]) {
			tokenStart := i
			i++
			for i < len(query) && isGraphQLNameContinue(query[i]) {
				i++
			}
			switch query[tokenStart:i] {
			case "query", "mutation", "subscription":
				operations++
				if operations > 1 {
					return false
				}
				pendingDefinitionBody = true
			case "fragment":
				pendingDefinitionBody = true
			}
			continue
		}
		i++
	}
	return operations == 1
}

func skipGraphQLLineComment(query string, i int) int {
	for i < len(query) && query[i] != '\n' && query[i] != '\r' {
		i++
	}
	return i
}

func skipGraphQLString(query string, i int) (int, bool) {
	if strings.HasPrefix(query[i:], `"""`) {
		end := strings.Index(query[i+3:], `"""`)
		if end < 0 {
			return len(query), false
		}
		return i + 3 + end + 3, true
	}
	i++
	for i < len(query) {
		switch query[i] {
		case '\\':
			i += 2
			continue
		case '"':
			return i + 1, true
		default:
			i++
		}
	}
	return len(query), false
}

func isGraphQLNameStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isGraphQLNameContinue(ch byte) bool {
	return isGraphQLNameStart(ch) || (ch >= '0' && ch <= '9')
}

func hasGraphQLErrors(payload map[string]any) bool {
	errorsValue, ok := payload["errors"]
	if !ok {
		return false
	}
	errorsList, ok := errorsValue.([]any)
	return ok && len(errorsList) > 0
}

func encodeDynamicToolPayload(payload any) string {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprint(payload)
	}
	return string(raw)
}
