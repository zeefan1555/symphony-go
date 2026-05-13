package logging

import "testing"

func TestSourceFieldsReturnsCallerLocation(t *testing.T) {
	fields := sourceFieldsProbe()

	if fields["source_file"] != "internal/runtime/logging/source_test.go" {
		t.Fatalf("source fields = %#v, want source_test.go", fields)
	}
	if line, ok := fields["source_line"].(int); !ok || line <= 0 {
		t.Fatalf("source fields = %#v, want positive source_line", fields)
	}
	if fields["source_function"] != "internal/runtime/logging.sourceFieldsProbe" {
		t.Fatalf("source fields = %#v, want sourceFieldsProbe", fields)
	}
}

func sourceFieldsProbe() map[string]any {
	return SourceFields(0)
}
