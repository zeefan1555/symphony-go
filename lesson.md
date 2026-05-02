# Lessons

## 2026-05-02: macOS `missing LC_UUID load command`

### 用户纠正

- 用户指出：不要每次都因为同一个 Go 测试命令踩 `missing LC_UUID load command`，应该写固定脚本，以后每次跑脚本。
- 用户要求：根目录 `build.sh` / `test.sh` 要高内聚，不要依赖隐藏的二级脚本。

### 错误模式

- 这是流程错误：仓库 `Makefile` 已经有 `-linkmode=external` 约定，但我手动运行裸 `go test`，绕过了仓库入口。
- 这是环境判断错误：`internal/linear` 的失败来自本机 macOS dyld 对 Go 临时测试二进制的加载限制，不是测试断言失败。
- 这是脚本设计错误：根目录入口如果只是跳到 `scripts/test.sh`，核心逻辑仍然不够高内聚，后续维护容易分叉。

### 防复犯规则

- 本仓 Go 测试不要直接运行裸 `go test`，除非目标就是验证裸 Go 工具链行为。
- 根目录 `build.sh` / `test.sh` 必须自包含关键逻辑：定位 repo root、设置 external linker、执行对应 Go 命令。
- 面向人和 AI 的稳定入口优先放在根目录；`scripts/` 只放内部辅助脚本，不承载必须记住的核心入口。

### 固定动作

- 跑完整测试：

```bash
./test.sh
```

- 跑局部包或单测：

```bash
./test.sh ./internal/orchestrator ./internal/linear
./test.sh ./internal/orchestrator -run TestName
```

- 构建：

```bash
./build.sh
```

- Makefile 也必须委托到根目录入口：`make test` 调 `./test.sh`，`make build` 调 `./build.sh`。
