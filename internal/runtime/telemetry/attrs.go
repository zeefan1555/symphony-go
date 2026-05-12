package telemetry

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

var metricLabelBlocklist = map[string]bool{
	"issue_id":         true,
	"issue_identifier": true,
	"session_id":       true,
	"thread_id":        true,
	"turn_id":          true,
	"workspace_path":   true,
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
