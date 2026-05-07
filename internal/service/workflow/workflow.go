package workflow

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/osteele/liquid"
	"gopkg.in/yaml.v3"
	runtimeconfig "symphony-go/internal/runtime/config"
	issuemodel "symphony-go/internal/service/issue"
)

const (
	ErrMissingWorkflowFile      = "missing_workflow_file"
	ErrWorkflowParse            = "workflow_parse_error"
	ErrWorkflowFrontMatterShape = "workflow_front_matter_not_a_map"
	ErrTemplateParse            = "template_parse_error"
	ErrTemplateRender           = "template_render_error"
)

type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Code + ": " + e.Message
	}
	return e.Code + ": " + e.Message + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error {
	return e.Err
}

func Code(err error) string {
	var workflowErr *Error
	if errors.As(err, &workflowErr) {
		return workflowErr.Code
	}
	return ""
}

const defaultPromptTemplate = `You are working on a Linear issue.

Identifier: {{ issue.identifier }}
Title: {{ issue.title }}

Body:
{% if issue.description %}
{{ issue.description }}
{% else %}
No description provided.
{% endif %}`

func Load(path string) (*runtimeconfig.Workflow, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, &Error{Code: ErrMissingWorkflowFile, Message: "read workflow", Err: err}
	}
	cfgBytes, prompt := splitFrontMatter(content)
	var cfg runtimeconfig.Config
	if len(bytes.TrimSpace(cfgBytes)) > 0 {
		var node yaml.Node
		if err := yaml.Unmarshal(cfgBytes, &node); err != nil {
			return nil, &Error{Code: ErrWorkflowParse, Message: "parse workflow front matter", Err: err}
		}
		if !frontMatterIsMap(node) {
			return nil, &Error{Code: ErrWorkflowFrontMatterShape, Message: "workflow front matter must be a YAML mapping"}
		}
		if err := node.Decode(&cfg); err != nil {
			return nil, &Error{Code: ErrWorkflowParse, Message: "parse workflow front matter", Err: err}
		}
	}
	resolved, err := runtimeconfig.Resolve(cfg, path)
	if err != nil {
		return nil, err
	}
	return &runtimeconfig.Workflow{Config: resolved, PromptTemplate: strings.TrimSpace(string(prompt))}, nil
}

func Render(template string, issue issuemodel.Issue, attempt *int) (string, error) {
	source := template
	if strings.TrimSpace(source) == "" {
		source = defaultPromptTemplate
	}

	engine := liquid.NewEngine()
	engine.StrictVariables()
	parsed, err := engine.ParseString(source)
	if err != nil {
		return "", &Error{Code: ErrTemplateParse, Message: fmt.Sprintf("parse template %q", source), Err: err}
	}
	rendered, err := parsed.RenderString(renderBindings(issue, attempt))
	if err != nil {
		return "", &Error{Code: ErrTemplateRender, Message: "render template", Err: err}
	}
	return rendered, nil
}

func frontMatterIsMap(node yaml.Node) bool {
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		return node.Content[0].Kind == yaml.MappingNode
	}
	return node.Kind == yaml.MappingNode
}

func splitFrontMatter(content []byte) ([]byte, []byte) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, content
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return []byte(strings.Join(lines[1:i], "\n")), []byte(strings.Join(lines[i+1:], "\n"))
		}
	}
	return []byte(strings.Join(lines[1:], "\n")), nil
}

func renderBindings(issue issuemodel.Issue, attempt *int) liquid.Bindings {
	bindings := liquid.Bindings{
		"attempt": nil,
		"issue":   issueBinding(issue),
	}
	if attempt != nil {
		bindings["attempt"] = *attempt
	}
	return bindings
}

func issueBinding(issue issuemodel.Issue) map[string]any {
	blockers := make([]map[string]any, 0, len(issue.BlockedBy))
	for _, blocker := range issue.BlockedBy {
		blockers = append(blockers, map[string]any{
			"id":         blocker.ID,
			"identifier": blocker.Identifier,
			"state":      blocker.State,
		})
	}
	return map[string]any{
		"id":          issue.ID,
		"identifier":  issue.Identifier,
		"title":       issue.Title,
		"description": optionalString(issue.Description),
		"priority":    optionalInt(issue.Priority),
		"state":       issue.State,
		"branch_name": optionalString(issue.BranchName),
		"url":         optionalString(issue.URL),
		"labels":      issue.Labels,
		"blocked_by":  blockers,
		"created_at":  optionalTime(issue.CreatedAt),
		"updated_at":  optionalTime(issue.UpdatedAt),
	}
}

func optionalString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func optionalInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}
