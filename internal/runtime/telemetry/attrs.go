package telemetry

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

var metricLabelBlocklist = map[string]bool{
	"additions":                   true,
	"changed_file_count":          true,
	"changed_lines":               true,
	"command_count":               true,
	"command_duration_ms":         true,
	"completed_at":                true,
	"command":                     true,
	"cwd":                         true,
	"deletions":                   true,
	"dominant_command_kind":       true,
	"evidence_file":               true,
	"evidence_line":               true,
	"evidence_locations":          true,
	"failed_command_count":        true,
	"file":                        true,
	"file_change_count":           true,
	"file_count":                  true,
	"file_locations":              true,
	"files":                       true,
	"final_message_present":       true,
	"message":                     true,
	"duration_ms":                 true,
	"issue_id":                    true,
	"issue_identifier":            true,
	"line_end":                    true,
	"line_start":                  true,
	"non_command_duration_ms":     true,
	"session_id":                  true,
	"slowest_command_duration_ms": true,
	"source_file":                 true,
	"source_function":             true,
	"source_line":                 true,
	"started_at":                  true,
	"search_command_count":        true,
	"read_command_count":          true,
	"test_command_count":          true,
	"build_command_count":         true,
	"git_command_count":           true,
	"other_command_count":         true,
	"threshold_ms":                true,
	"thread_id":                   true,
	"turn_count":                  true,
	"turn_id":                     true,
	"workspace_path":              true,
}

func Attrs(fields map[string]any) []attribute.KeyValue {
	if len(fields) == 0 {
		return nil
	}
	attrs := make([]attribute.KeyValue, 0, len(fields))
	for key, value := range fields {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				attrs = append(attrs, attribute.String(key, typed))
			}
		case int:
			attrs = append(attrs, attribute.Int(key, typed))
		case int64:
			attrs = append(attrs, attribute.Int64(key, typed))
		case bool:
			attrs = append(attrs, attribute.Bool(key, typed))
		default:
			attrs = append(attrs, attribute.String(key, fmt.Sprint(typed)))
		}
	}
	return attrs
}

func MetricAttrs(fields map[string]any) []attribute.KeyValue {
	if len(fields) == 0 {
		return nil
	}
	filtered := make(map[string]any, len(fields))
	for key, value := range fields {
		if metricLabelBlocklist[strings.TrimSpace(key)] {
			continue
		}
		filtered[key] = value
	}
	return Attrs(filtered)
}
