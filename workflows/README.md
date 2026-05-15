# Workflow Registry

这个目录用于集中管理本机/多仓的 Symphony workflow prompt。`symphony-go` 的运行脚本和 CLI 默认读取 `workflows/WORKFLOW-symphony-go.md`；本目录是长期维护入口，后续新增或调整 workflow 时优先更新这里。

## 文件索引

| 文件 | 来源 | 用途 |
| --- | --- | --- |
| `WORKFLOW-symphony-go.md` | 原 `/Users/bytedance/symphony-go/WORKFLOW.md` | `symphony-go` 主仓无人值守 Linear issue 实现、AI Review、Merging workflow。 |
| `WORKFLOW-bytedcode.md` | `/Users/bytedance/bytecode/WORKFLOW.md` | `/Users/bytedance/bytecode` 聚合目录只读诊断 workflow，用于跨仓定位、bytedcli 取证和 Linear workpad 收口。 |
| `WORKFLOW-social-pet.md` | 原 `/Users/bytedance/bytecode/Backend-Server/social_pet/WORKFLOW.md` | `ttgame/social_pet` 专用问题排查 workflow，用于源码、日志、trace、TCC/CDS 和局部测试证据链。 |
| `WORKFLOW-zeefan-explore.md` | `/Users/bytedance/zeefan-explore/AGENTS.md` | `/Users/bytedance/zeefan-explore` 探索 workflow，用于 Twitter、ByteTech、工具安装试用和技能实验的 issue 驱动收口。 |

## 维护规则

1. 新 workflow 统一放在本目录，命名使用 `WORKFLOW-<scope>.md` 或 `WORKFLOW-<scope>-<purpose>.md`。
2. 文件内容应保持可直接被 Symphony runner 读取：顶部 frontmatter + prompt 正文。
3. 修改已投入运行的 workflow 时，同时说明是否需要同步回原运行路径。
4. 不在这里保存 issue-specific 临时 prompt、workpad 内容、日志大文件或 secret。
5. 迁入外部 workflow 时保留来源路径，便于 diff 和回写。

## 运行入口

本仓默认运行入口：

```bash
make run
make run-once ISSUE=<issue>
make bytecode-triage
make bytecode-triage-once ISSUE=<issue>
make explore
make explore-once ISSUE=<issue>
```

等价显式命令：

```bash
./bin/symphony-go run --workflow ./workflows/WORKFLOW-symphony-go.md --once --no-tui --issue <issue>
./bin/symphony-go run --workflow ./workflows/WORKFLOW-bytedcode.md --once --no-tui --issue <issue>
./bin/symphony-go run --workflow ./workflows/WORKFLOW-zeefan-explore.md --once --no-tui --issue <issue>
```
