# Goal: SPEC gap analysis and staged Go implementation optimization

## Original Request

分析本仓库 Go 语言实现版本与 `SPEC.md` 的差距，然后优化本仓库的代码，分阶段提交 commit 到 `main` 分支上。

## Interpreted Outcome

Use GoalBuddy to run a controlled, evidence-driven execution tranche that:

- maps current Go implementation behavior against `SPEC.md`;
- identifies verified gaps and prioritizes the safest high-value implementation slices;
- applies minimal, staged code changes;
- verifies each stage with repo-native checks;
- commits each verified stage on `main` only when the branch, dirty state, and verification evidence are safe.

## Input Shape

specific execution goal with audit-first discovery and staged implementation.

## Non-Negotiable Constraints

- `SPEC.md` is the primary behavior contract for this goal.
- Do not rely on stale documentation when live code and `SPEC.md` disagree.
- Keep edits minimal and directly tied to verified spec gaps.
- Use the repository's own verification entrypoints, especially `./test.sh` and `./build.sh`, for Go changes.
- Preserve unrelated dirty work; never revert or overwrite user/other-agent changes.
- Commit in stages only after the relevant slice is verified.
- Before committing to `main`, confirm the active branch and working tree state make that safe.

## Likely Misfire To Avoid

Avoid producing only an audit or plan without implementing safe verified slices. Also avoid directly changing code without spec-to-code evidence and staged verification receipts.

## Completion Proof

The goal is complete only when a final audit maps implemented commits and verification receipts back to the original request and confirms no required local, safe spec-gap implementation slice remains for the current tranche.

## Starter Command

```text
/goal Follow docs/goals/spec-gap-code-optimization/goal.md.
```
