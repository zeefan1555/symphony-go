package workflow

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/zeefan1555/symphony-go/internal/config"
	"github.com/zeefan1555/symphony-go/internal/types"
	"gopkg.in/yaml.v3"
)

var variablePattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)
var attemptBlockPattern = regexp.MustCompile(`(?s)\{%[[:space:]]*if[[:space:]]+attempt[[:space:]]*%\}(.*?)\{%[[:space:]]*endif[[:space:]]*%\}`)

func Load(path string) (*types.Workflow, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow: %w", err)
	}
	cfgBytes, prompt := splitFrontMatter(content)
	var cfg types.Config
	if len(bytes.TrimSpace(cfgBytes)) > 0 {
		if err := yaml.Unmarshal(cfgBytes, &cfg); err != nil {
			return nil, fmt.Errorf("parse workflow front matter: %w", err)
		}
	}
	resolved, err := config.Resolve(cfg, path)
	if err != nil {
		return nil, err
	}
	return &types.Workflow{Config: resolved, PromptTemplate: strings.TrimSpace(string(prompt))}, nil
}

func Render(template string, issue types.Issue, attempt *int) (string, error) {
	rendered := attemptBlockPattern.ReplaceAllStringFunc(template, func(match string) string {
		if attempt == nil {
			return ""
		}
		parts := attemptBlockPattern.FindStringSubmatch(match)
		if len(parts) == 2 {
			return parts[1]
		}
		return match
	})

	missing := ""
	rendered = variablePattern.ReplaceAllStringFunc(rendered, func(match string) string {
		key := variablePattern.FindStringSubmatch(match)[1]
		value, ok := templateValue(key, issue, attempt)
		if !ok {
			missing = key
			return match
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("unknown template variable %q", missing)
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

func templateValue(key string, issue types.Issue, attempt *int) (string, bool) {
	switch key {
	case "attempt":
		if attempt == nil {
			return "", true
		}
		return fmt.Sprintf("%d", *attempt), true
	case "issue.id":
		return issue.ID, true
	case "issue.identifier":
		return issue.Identifier, true
	case "issue.title":
		return issue.Title, true
	case "issue.state":
		return issue.State, true
	case "issue.labels":
		return strings.Join(issue.Labels, ", "), true
	case "issue.url":
		return issue.URL, true
	case "issue.description":
		return issue.Description, true
	default:
		return "", false
	}
}
