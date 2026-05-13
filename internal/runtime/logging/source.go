package logging

import (
	"path/filepath"
	"runtime"
	"strings"
)

func SourceFields(skip int) map[string]any {
	pc, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return nil
	}
	fields := map[string]any{
		"source_file": filepath.ToSlash(trimRepoPath(file)),
		"source_line": line,
	}
	if fn := runtime.FuncForPC(pc); fn != nil {
		fields["source_function"] = trimFunctionName(fn.Name())
	}
	return fields
}

func trimRepoPath(path string) string {
	path = filepath.ToSlash(path)
	if index := strings.Index(path, "/internal/"); index >= 0 {
		return strings.TrimPrefix(path[index:], "/")
	}
	return path
}

func trimFunctionName(name string) string {
	const module = "symphony-go/"
	if index := strings.Index(name, module); index >= 0 {
		return name[index+len(module):]
	}
	return name
}
