package ssh

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const ConfigEnv = "SYMPHONY_SSH_CONFIG"

var ErrNotFound = errors.New("ssh not found")

var hostPortPattern = regexp.MustCompile(`^(.*):(\d+)$`)

type Result struct {
	Output   string
	ExitCode int
}

type RunOptions struct {
	Env            []string
	Dir            string
	StderrToStdout bool
}

type StartOptions struct {
	Env []string
	Dir string
}

type Port struct {
	Cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
}

func Run(ctx context.Context, host string, command string, opts RunOptions) (Result, error) {
	executable, err := executable()
	if err != nil {
		return Result{}, err
	}
	cmd := exec.CommandContext(ctx, executable, Args(host, command)...)
	cmd.Dir = opts.Dir
	cmd.Env = env(opts.Env)
	output, err := runOutput(cmd, opts.StderrToStdout)
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return Result{Output: string(output), ExitCode: exitCode}, nil
		}
		return Result{Output: string(output), ExitCode: exitCode}, err
	}
	return Result{Output: string(output), ExitCode: exitCode}, nil
}

func StartPort(ctx context.Context, host string, command string, opts StartOptions) (*Port, error) {
	executable, err := executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, executable, Args(host, command)...)
	cmd.Dir = opts.Dir
	cmd.Env = env(opts.Env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Port{Cmd: cmd, Stdin: stdin, Stdout: stdout}, nil
}

func Args(host string, command string) []string {
	target := ParseTarget(host)
	args := make([]string, 0, 7)
	if configPath := strings.TrimSpace(os.Getenv(ConfigEnv)); configPath != "" {
		args = append(args, "-F", configPath)
	}
	args = append(args, "-T")
	if target.Port != "" {
		args = append(args, "-p", target.Port)
	}
	args = append(args, target.Destination, RemoteShellCommand(command))
	return args
}

type Target struct {
	Destination string
	Port        string
}

func ParseTarget(target string) Target {
	trimmed := strings.TrimSpace(target)
	matches := hostPortPattern.FindStringSubmatch(trimmed)
	if len(matches) == 3 && validPortDestination(matches[1]) {
		return Target{Destination: matches[1], Port: matches[2]}
	}
	return Target{Destination: trimmed}
}

func RemoteShellCommand(command string) string {
	return "bash -lc " + shellEscape(command)
}

func executable() (string, error) {
	path, err := exec.LookPath("ssh")
	if err != nil {
		return "", ErrNotFound
	}
	return path, nil
}

func runOutput(cmd *exec.Cmd, stderrToStdout bool) ([]byte, error) {
	if stderrToStdout {
		return cmd.CombinedOutput()
	}
	return cmd.Output()
}

func env(extra []string) []string {
	if len(extra) == 0 {
		return os.Environ()
	}
	return append(os.Environ(), extra...)
}

func validPortDestination(destination string) bool {
	return destination != "" && (!strings.Contains(destination, ":") || bracketedHost(destination))
}

func bracketedHost(destination string) bool {
	return strings.Contains(destination, "[") && strings.Contains(destination, "]")
}

func shellEscape(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
