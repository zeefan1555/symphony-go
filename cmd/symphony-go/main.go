package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"symphony-go/internal/app"
)

const defaultWorkflowPath = "workflow.md"

type runOptions struct {
	WorkflowPath       string
	workflowExplicit   bool
	Once               bool
	Issue              string
	MergeTarget        string
	mergeExplicit      bool
	ServerPort         int
	serverPortExplicit bool
	TUI                bool
	tuiExplicit        bool
}

func defaultRunOptions() runOptions {
	return runOptions{
		WorkflowPath: defaultWorkflowPath,
		MergeTarget:  "main",
		TUI:          true,
	}
}

func parseRunOptions(args []string) (runOptions, error) {
	opts := defaultRunOptions()
	runFlags := flag.NewFlagSet("run", flag.ContinueOnError)
	runFlags.StringVar(&opts.WorkflowPath, "workflow", opts.WorkflowPath, "path to workflow file")
	runFlags.BoolVar(&opts.Once, "once", opts.Once, "poll once and exit")
	runFlags.StringVar(&opts.Issue, "issue", opts.Issue, "optional issue identifier or id filter")
	runFlags.StringVar(&opts.MergeTarget, "merge-target", opts.MergeTarget, "local branch receiving Merging-state work")
	runFlags.IntVar(&opts.ServerPort, "port", opts.ServerPort, "start local HTTP control plane on this port")
	runFlags.Var(tuiFlag{opts: &opts, enabled: true}, "tui", "render terminal TUI")
	runFlags.Var(tuiFlag{opts: &opts, enabled: false}, "no-tui", "disable terminal TUI")
	if err := runFlags.Parse(args); err != nil {
		return runOptions{}, err
	}
	runFlags.Visit(func(f *flag.Flag) {
		if f.Name == "workflow" {
			opts.workflowExplicit = true
		}
		if f.Name == "merge-target" {
			opts.mergeExplicit = true
		}
		if f.Name == "port" {
			opts.serverPortExplicit = true
		}
	})
	if opts.serverPortExplicit && opts.ServerPort < 0 {
		return runOptions{}, fmt.Errorf("port must be zero or positive")
	}
	positionals := runFlags.Args()
	if len(positionals) > 1 {
		return runOptions{}, fmt.Errorf("expected at most one workflow path argument")
	}
	if len(positionals) == 1 && !opts.workflowExplicit {
		opts.WorkflowPath = positionals[0]
	}
	opts.ApplyDefaults()
	return opts, nil
}

func (o *runOptions) ApplyDefaults() {
	if o.Once && !o.tuiExplicit {
		o.TUI = false
	}
}

type tuiFlag struct {
	opts    *runOptions
	enabled bool
}

func (f tuiFlag) String() string {
	if f.opts == nil || !f.opts.TUI {
		return "false"
	}
	return "true"
}

func (f tuiFlag) Set(value string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	if parsed {
		f.opts.TUI = f.enabled
	} else {
		f.opts.TUI = !f.enabled
	}
	f.opts.tuiExplicit = true
	return nil
}

func (f tuiFlag) IsBoolFlag() bool {
	return true
}

func main() {
	os.Exit(runMain(os.Args, app.RunWithSignals))
}

func runMain(args []string, run func(app.Options) error) int {
	if len(args) < 2 {
		printUsage()
		return 2
	}
	switch args[1] {
	case "init":
		return runInitMain(args[2:])
	case "run":
	default:
		printUsage()
		return 2
	}

	opts, err := parseRunOptions(args[2:])
	if err != nil {
		return 2
	}

	if err := run(opts.AppOptions()); err != nil {
		fmt.Fprintln(os.Stderr, "symphony-go:", err)
		return 1
	}
	return 0
}

func mergeTargetOverride(opts runOptions) string {
	return app.MergeTargetOverride(opts.MergeTarget, opts.mergeExplicit)
}

func (o runOptions) AppOptions() app.Options {
	return app.Options{
		WorkflowPath:  o.WorkflowPath,
		Once:          o.Once,
		Issue:         o.Issue,
		MergeTarget:   o.MergeTarget,
		MergeExplicit: o.mergeExplicit,
		Server: app.ServerOptions{
			Port:         o.ServerPort,
			PortExplicit: o.serverPortExplicit,
		},
		TUI:    o.TUI,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  symphony-go init [--output workflow.md] [--project-slug slug] [--merge-target main] [--force]")
	fmt.Fprintln(os.Stderr, "  symphony-go run [path-to-workflow.md] [--workflow workflow.md] [--once] [--issue ZEE-8] [--port 0] [--tui|--no-tui]")
}

type initOptions struct {
	Output      string
	Dir         string
	ProjectSlug string
	MergeTarget string
	Force       bool
}

type localRepoInfo struct {
	Root          string
	ProjectName   string
	RemoteURL     string
	DefaultBranch string
	CurrentBranch string
	HasAgentsMD   bool
	HasClaudeMD   bool
	Stacks        []detectedStack
}

type detectedStack struct {
	Name     string
	Commands []string
}

func defaultInitOptions() initOptions {
	return initOptions{
		Output: defaultWorkflowPath,
		Dir:    ".",
	}
}

func parseInitOptions(args []string) (initOptions, error) {
	opts := defaultInitOptions()
	initFlags := flag.NewFlagSet("init", flag.ContinueOnError)
	initFlags.StringVar(&opts.Output, "output", opts.Output, "workflow file to create")
	initFlags.StringVar(&opts.Dir, "dir", opts.Dir, "repository directory to initialize")
	initFlags.StringVar(&opts.ProjectSlug, "project-slug", opts.ProjectSlug, "Linear project slug")
	initFlags.StringVar(&opts.MergeTarget, "merge-target", opts.MergeTarget, "target branch for merge-stage work")
	initFlags.BoolVar(&opts.Force, "force", opts.Force, "overwrite an existing workflow file")
	if err := initFlags.Parse(args); err != nil {
		return initOptions{}, err
	}
	if strings.TrimSpace(opts.Output) == "" {
		return initOptions{}, fmt.Errorf("output must be non-empty")
	}
	if strings.TrimSpace(opts.Dir) == "" {
		return initOptions{}, fmt.Errorf("dir must be non-empty")
	}
	if extra := initFlags.Args(); len(extra) > 0 {
		return initOptions{}, fmt.Errorf("unexpected init argument %q", extra[0])
	}
	return opts, nil
}

func runInitMain(args []string) int {
	opts, err := parseInitOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "symphony-go init:", err)
		return 2
	}
	if err := writeInitialWorkflow(opts); err != nil {
		fmt.Fprintln(os.Stderr, "symphony-go init:", err)
		return 1
	}
	return 0
}

func writeInitialWorkflow(opts initOptions) error {
	info := scanLocalRepo(opts.Dir)
	outputPath := resolveInitOutputPath(opts.Dir, opts.Output)
	if _, err := os.Stat(outputPath); err == nil && !opts.Force {
		return fmt.Errorf("%s already exists; use --force to overwrite", outputPath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	content := generateInitialWorkflow(opts, info)
	if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "symphony-go init: wrote %s\n", outputPath)
	fmt.Fprintln(os.Stdout, "next:")
	if filepath.Clean(opts.Output) == defaultWorkflowPath {
		fmt.Fprintln(os.Stdout, "  symphony-go run")
	} else {
		fmt.Fprintf(os.Stdout, "  symphony-go run --workflow %s\n", outputPath)
	}
	return nil
}

func resolveInitOutputPath(dir string, output string) string {
	if filepath.IsAbs(output) {
		return filepath.Clean(output)
	}
	return filepath.Clean(filepath.Join(dir, output))
}

func scanLocalRepo(dir string) localRepoInfo {
	root := dir
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		root = strings.TrimSpace(string(out))
	}
	rootAbs, err := filepath.Abs(root)
	if err == nil {
		root = filepath.Clean(rootAbs)
	}
	info := localRepoInfo{
		Root:          root,
		ProjectName:   filepath.Base(root),
		DefaultBranch: "main",
	}
	if out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output(); err == nil {
		info.RemoteURL = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD").Output(); err == nil {
		info.DefaultBranch = strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/")
	} else if out, err := exec.Command("git", "-C", dir, "branch", "--show-current").Output(); err == nil {
		if branch := strings.TrimSpace(string(out)); branch != "" {
			info.DefaultBranch = branch
		}
	}
	if out, err := exec.Command("git", "-C", dir, "branch", "--show-current").Output(); err == nil {
		info.CurrentBranch = strings.TrimSpace(string(out))
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err == nil {
		info.HasAgentsMD = true
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err == nil {
		info.HasClaudeMD = true
	}
	info.Stacks = detectStacks(root)
	return info
}

func detectStacks(dir string) []detectedStack {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	var stacks []detectedStack
	if has("go.mod") {
		commands := []string{"go test ./...", "go vet ./..."}
		if has("test.sh") {
			commands[0] = "./test.sh ./..."
		}
		if has("build.sh") {
			commands = append(commands, "./build.sh")
		}
		stacks = append(stacks, detectedStack{Name: "Go", Commands: commands})
	}
	if has("package.json") {
		stacks = append(stacks, detectedStack{Name: "Node.js", Commands: []string{"npm test", "npm run build"}})
	}
	if has("pyproject.toml") || has("requirements.txt") {
		stacks = append(stacks, detectedStack{Name: "Python", Commands: []string{"python -m pytest"}})
	}
	return stacks
}

func generateInitialWorkflow(opts initOptions, info localRepoInfo) string {
	projectSlug := strings.TrimSpace(opts.ProjectSlug)
	if projectSlug == "" {
		projectSlug = os.Getenv("LINEAR_PROJECT_SLUG")
	}
	if projectSlug == "" {
		projectSlug = safeProjectSlug(info.ProjectName)
	}
	mergeTarget := strings.TrimSpace(opts.MergeTarget)
	if mergeTarget == "" {
		mergeTarget = info.DefaultBranch
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("tracker:\n")
	b.WriteString("  kind: linear\n")
	b.WriteString("  api_key: $LINEAR_API_KEY\n")
	b.WriteString("  project_slug: " + yamlString(projectSlug) + "\n")
	b.WriteString("  active_states:\n")
	for _, state := range []string{"Todo", "In Progress", "AI Review", "Pushing", "Merging", "Rework"} {
		b.WriteString("    - " + yamlString(state) + "\n")
	}
	b.WriteString("  terminal_states:\n")
	for _, state := range []string{"Closed", "Cancelled", "Canceled", "Duplicate", "Done"} {
		b.WriteString("    - " + yamlString(state) + "\n")
	}
	b.WriteString("polling:\n")
	b.WriteString("  interval_ms: 5000\n")
	b.WriteString("workspace:\n")
	b.WriteString("  mode: static_cwd\n")
	b.WriteString("  cwd: .\n")
	b.WriteString("merge:\n")
	b.WriteString("  target: " + yamlString(mergeTarget) + "\n")
	b.WriteString("agent:\n")
	b.WriteString("  max_concurrent_agents: 3\n")
	b.WriteString("  max_turns: 20\n")
	b.WriteString("  review_policy:\n")
	b.WriteString("    mode: auto\n")
	b.WriteString("    allow_manual_ai_review: false\n")
	b.WriteString("    on_ai_fail: rework\n")
	b.WriteString("codex:\n")
	b.WriteString("  command: codex --config shell_environment_policy.inherit=all app-server\n")
	b.WriteString("  read_timeout_ms: 60000\n")
	b.WriteString("  approval_policy: never\n")
	b.WriteString("  thread_sandbox: workspace-write\n")
	b.WriteString("  turn_sandbox_policy:\n")
	b.WriteString("    type: workspaceWrite\n")
	b.WriteString("    readOnlyAccess:\n")
	b.WriteString("      type: fullAccess\n")
	b.WriteString("    networkAccess: true\n")
	b.WriteString("    excludeTmpdirEnvVar: false\n")
	b.WriteString("    excludeSlashTmp: false\n")
	b.WriteString("---\n\n")
	b.WriteString("你正在处理 Linear ticket `{{ issue.identifier }}`。\n\n")
	b.WriteString("## 仓库上下文\n\n")
	b.WriteString("- 当前仓库：" + info.ProjectName + "\n")
	if info.RemoteURL != "" {
		b.WriteString("- Git remote：" + info.RemoteURL + "\n")
	}
	b.WriteString("- 目标分支：" + mergeTarget + "\n\n")
	b.WriteString("## 执行规则\n\n")
	b.WriteString("1. 先读取 ticket，再查看相关代码和本仓约定，确认成功标准后再修改。\n")
	b.WriteString("2. 只在当前 repo root 内工作，不创建额外 clone 或临时 checkout。\n")
	b.WriteString("3. 保持最小改动；不要顺手重构、格式化无关文件或提交无关脏区。\n")
	b.WriteString("4. 完成前运行与改动直接相关的验证，并在最终回复里列出命令和结果。\n")
	if info.HasAgentsMD {
		b.WriteString("5. 本仓存在 `AGENTS.md`，开始前必须阅读并遵守。\n")
	} else if info.HasClaudeMD {
		b.WriteString("5. 本仓存在 `CLAUDE.md`，开始前必须阅读并遵守。\n")
	}
	if len(info.Stacks) > 0 {
		b.WriteString("\n## 可优先尝试的验证命令\n\n")
		b.WriteString("```bash\n")
		for _, stack := range info.Stacks {
			b.WriteString("# " + stack.Name + "\n")
			for _, cmd := range stack.Commands {
				b.WriteString(cmd + "\n")
			}
		}
		b.WriteString("```\n")
	}
	return b.String()
}

func safeProjectSlug(projectName string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(projectName) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "project-slug"
	}
	return slug
}

func yamlString(value string) string {
	return strconv.Quote(value)
}
