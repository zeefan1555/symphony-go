# Goal: Elixir to Go feature parity

## Original Request

分析本仓库 Go 语言实现版本与 Elixir 实现的差距，然后把 Elixir 的所有功能都复刻到 Go 语言项目中。

## Interpreted Outcome

Use GoalBuddy to run a controlled parity execution line that:

- maps the Elixir reference implementation and current Go implementation into a source-backed feature inventory;
- distinguishes required parity features from legacy experiments, non-goals, already-superseded behavior, and unsafe scope;
- ports required missing Elixir behavior to the Go project in small verified slices;
- keeps each slice minimal, tested, and compatible with this repository's current architecture;
- records durable receipts so a later run can continue without rediscovering the same parity map.

## Input Shape

open-ended execution goal with audit-first discovery and staged implementation.

## Non-Negotiable Constraints

- Do not copy Elixir behavior blindly; first prove whether each behavior is required, already implemented, intentionally different, obsolete, or out of scope.
- Live Go code and the Elixir reference implementation are the evidence sources for parity mapping.
- `SPEC.md`, `AGENTS.md`, existing repo architecture boundaries, and current Go verification scripts remain binding constraints.
- Preserve unrelated dirty work; never revert or stage user or other-agent changes.
- Use minimal, staged Worker slices with explicit `allowed_files`, verification commands, and stop conditions.
- For Go changes, prefer `./test.sh` and `./build.sh` rather than bare `go test` or `go build`.

## Likely Misfire To Avoid

Avoid treating "all Elixir functionality" as permission to import obsolete, experimental, or non-goal behavior into Go. Also avoid stopping after an audit without implementing safe, verified parity slices.

## Completion Proof

The goal is complete only when a final audit maps the Elixir feature inventory to Go implementation evidence and shows that every required parity feature is either implemented and verified, explicitly judged already equivalent, or blocked/out-of-scope with a receipt explaining why.

## Starter Command

```text
/goal Follow docs/goals/elixir-go-parity/goal.md.
```
