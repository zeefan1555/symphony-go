package workflow

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/osteele/liquid"
	"gopkg.in/yaml.v3"
	runtimeconfig "symphony-go/internal/runtime/config"
	issuemodel "symphony-go/internal/service/issue"
)

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
		return nil, fmt.Errorf("read workflow: %w", err)
	}
	cfgBytes, prompt := splitFrontMatter(content)
	var cfg runtimeconfig.Config
	if len(bytes.TrimSpace(cfgBytes)) > 0 {
		if err := yaml.Unmarshal(cfgBytes, &cfg); err != nil {
			return nil, fmt.Errorf("parse workflow front matter: %w", err)
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
		return "", fmt.Errorf("template_parse_error: %w template=%q", err, source)
	}
	rendered, err := parsed.RenderString(renderBindings(issue, attempt))
	if err != nil {
		return "", fmt.Errorf("template_render_error: %w", err)
	}
	return rendered, nil
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
