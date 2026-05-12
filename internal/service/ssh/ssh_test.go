package ssh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunKeepsBracketedIPv6HostPortTargetsIntact(t *testing.T) {
	traceFile := installFakeSSH(t, "")

	result, err := Run(context.Background(), "root@[::1]:2200", "printf ok", RunOptions{StderrToStdout: true})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}

	trace := readTrace(t, traceFile)
	if !strings.Contains(trace, "-T -p 2200 root@[::1] bash -lc") {
		t.Fatalf("trace missing bracketed IPv6 target: %q", trace)
	}
	if !strings.Contains(trace, "printf ok") {
		t.Fatalf("trace missing command: %q", trace)
	}
}

func TestRunLeavesUnbracketedIPv6StyleTargetsUnchanged(t *testing.T) {
	traceFile := installFakeSSH(t, "")

	_, err := Run(context.Background(), "::1:2200", "printf ok", RunOptions{StderrToStdout: true})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	trace := readTrace(t, traceFile)
	if !strings.Contains(trace, "-T ::1:2200 bash -lc") {
		t.Fatalf("trace missing raw IPv6-style target: %q", trace)
	}
	if strings.Contains(trace, "-p 2200") {
		t.Fatalf("trace unexpectedly split unbracketed IPv6 port: %q", trace)
	}
}

func TestRunPassesHostPortTargetsThroughSSHPFlag(t *testing.T) {
	traceFile := installFakeSSH(t, "")
	t.Setenv(ConfigEnv, "/tmp/symphony-test-ssh-config")

	_, err := Run(context.Background(), "localhost:2222", "echo ready", RunOptions{StderrToStdout: true})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	trace := readTrace(t, traceFile)
	if !strings.Contains(trace, "-F /tmp/symphony-test-ssh-config") {
		t.Fatalf("trace missing ssh config: %q", trace)
	}
	if !strings.Contains(trace, "-T -p 2222 localhost bash -lc") {
		t.Fatalf("trace missing host:port split: %q", trace)
	}
	if !strings.Contains(trace, "echo ready") {
		t.Fatalf("trace missing command: %q", trace)
	}
}

func TestRunKeepsUserPrefixWhenParsingUserHostPortTargets(t *testing.T) {
	traceFile := installFakeSSH(t, "")

	_, err := Run(context.Background(), "root@127.0.0.1:2200", "printf ok", RunOptions{StderrToStdout: true})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	trace := readTrace(t, traceFile)
	if !strings.Contains(trace, "-T -p 2200 root@127.0.0.1 bash -lc") {
		t.Fatalf("trace missing user host:port split: %q", trace)
	}
}

func TestRunReturnsErrNotFoundWhenSSHIsUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := Run(context.Background(), "localhost", "printf ok", RunOptions{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Run error = %v, want ErrNotFound", err)
	}
}

func TestStartPortSupportsBinaryOutput(t *testing.T) {
	traceFile := installFakeSSH(t, "#!/bin/sh\nprintf 'ARGV:%s\\n' \"$*\" >> \"$TRACE_FILE\"\nprintf 'ready\\n'\nexit 0\n")
	t.Setenv(ConfigEnv, "")

	port, err := StartPort(context.Background(), "localhost", "printf ok", StartOptions{})
	if err != nil {
		t.Fatalf("StartPort returned error: %v", err)
	}
	defer port.Cmd.Wait()
	defer port.Stdin.Close()

	trace := waitForTrace(t, traceFile)
	if !strings.Contains(trace, "-T localhost bash -lc") {
		t.Fatalf("trace missing localhost command: %q", trace)
	}
	if strings.Contains(trace, " -F ") {
		t.Fatalf("trace unexpectedly included ssh config: %q", trace)
	}
}

func TestStartPortPassesHostPortTargetsThroughSSHPFlag(t *testing.T) {
	traceFile := installFakeSSH(t, "#!/bin/sh\nprintf 'ARGV:%s\\n' \"$*\" >> \"$TRACE_FILE\"\nprintf 'ready\\n'\nexit 0\n")

	port, err := StartPort(context.Background(), "localhost:2222", "printf ok", StartOptions{})
	if err != nil {
		t.Fatalf("StartPort returned error: %v", err)
	}
	defer port.Cmd.Wait()
	defer port.Stdin.Close()

	trace := waitForTrace(t, traceFile)
	if !strings.Contains(trace, "-T -p 2222 localhost bash -lc") {
		t.Fatalf("trace missing host:port split: %q", trace)
	}
}

func TestRemoteShellCommandEscapesEmbeddedSingleQuotes(t *testing.T) {
	got := RemoteShellCommand("printf 'hello'")
	want := "bash -lc 'printf '\"'\"'hello'\"'\"''"
	if got != want {
		t.Fatalf("RemoteShellCommand() = %q, want %q", got, want)
	}
}

func installFakeSSH(t *testing.T, script string) string {
	t.Helper()

	testRoot := t.TempDir()
	fakeBinDir := filepath.Join(testRoot, "bin")
	traceFile := filepath.Join(testRoot, "ssh.trace")
	if err := os.MkdirAll(fakeBinDir, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	if script == "" {
		script = "#!/bin/sh\nprintf 'ARGV:%s\\n' \"$*\" >> \"$TRACE_FILE\"\nexit 0\n"
	}
	fakeSSH := filepath.Join(fakeBinDir, "ssh")
	if err := os.WriteFile(fakeSSH, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("TRACE_FILE", traceFile)
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return traceFile
}

func readTrace(t *testing.T, traceFile string) string {
	t.Helper()

	data, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	return string(data)
}

func waitForTrace(t *testing.T, traceFile string) string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(traceFile)
		if err == nil && len(data) > 0 {
			return string(data)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for fake ssh trace at %s", traceFile)
	return ""
}
