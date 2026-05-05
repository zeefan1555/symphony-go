#!/usr/bin/env python3
"""Create and run one fixed Chinese smoke benchmark issue."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import urllib.request
from datetime import datetime
from pathlib import Path
from typing import Any


ISSUE_TEMPLATE = """中文冒烟测试：固定时间戳任务

请使用中文冒烟 benchmark workflow 处理：

- 只改 `docs/smoke/pr-merge-fast-path-smoke.md`
- 在文件末尾追加一组新的冒烟记录：
  - `Timestamp: {timestamp}`
  - `Issue: <本 issue identifier>`
  - `Note: PR merge fast path smoke; the change was made in the issue worktree and merged through the PR flow.`
- 运行 `rg -n "Issue: <本 issue identifier>" docs/smoke/pr-merge-fast-path-smoke.md`
- 运行 `git diff --check`
- 创建一个本地 commit
- 进入 `AI Review`，review 通过后进入 `Merging`
- 在 `Merging` 阶段通过 PR merge flow 创建或更新 PR、等待检查并 merge
- PR merge 完成后把 issue 移动到 `Done`，由框架自动清理 issue worktree
- 所有 Workpad、状态说明和最终回复使用中文
"""

def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run one zh-smoke benchmark round.")
    parser.add_argument("--workflow", default="./WORKFLOW.zh-smoke.md")
    parser.add_argument("--team", default=os.environ.get("ZH_SMOKE_TEAM", "Zeefan"))
    parser.add_argument("--state", default="Todo")
    parser.add_argument("--merge-target", default=os.environ.get("MERGE_TARGET", ""))
    parser.add_argument("--title", default="中文冒烟测试：固定时间戳任务")
    parser.add_argument("--results", default="../.codex/skills/zh-smoke-harness/experiments/results.tsv")
    parser.add_argument("--markdown", default="../.codex/skills/zh-smoke-harness/experiments/rounds.md")
    parser.add_argument("--change-note", default=os.environ.get("ZH_SMOKE_CHANGE_NOTE", ""))
    parser.add_argument("--dry-run", action="store_true", help="Resolve Linear targets but do not create or run.")
    parser.add_argument("--create-only", action="store_true", help="Create the Linear issue but do not run Symphony.")
    return parser.parse_args()


def workflow_value(path: Path, key: str) -> str:
    text = path.read_text(encoding="utf-8")
    match = re.search(rf"(?m)^\s*{re.escape(key)}:\s*\"?([^\"\n]+)\"?\s*$", text)
    value = match.group(1).strip() if match else ""
    if value.startswith("$") and "/" not in value and "\\" not in value and " " not in value:
        return os.environ.get(value[1:], "")
    return value


def graphql(endpoint: str, api_key: str, query: str, variables: dict[str, Any]) -> dict[str, Any]:
    payload = json.dumps({"query": query, "variables": variables}).encode("utf-8")
    request = urllib.request.Request(
        endpoint,
        data=payload,
        headers={
            "Authorization": api_key,
            "Content-Type": "application/json; charset=utf-8",
            "Accept": "application/json",
        },
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=30) as response:
        body = response.read().decode("utf-8")
    decoded = json.loads(body)
    if decoded.get("errors"):
        raise SystemExit(f"Linear GraphQL errors: {decoded['errors']}")
    return decoded.get("data", {})


def resolve_targets(endpoint: str, api_key: str, project_slug: str, team_name: str, state_name: str) -> dict[str, str]:
    data = graphql(
        endpoint,
        api_key,
        """
        query ZhSmokeResolve($projectSlug: String!) {
          projects(filter: {slugId: {eq: $projectSlug}}, first: 1) {
            nodes {
              id
              name
              teams(first: 50) {
                nodes {
                  id
                  key
                  name
                  states(first: 100) { nodes { id name } }
                }
              }
            }
          }
        }
        """,
        {"projectSlug": project_slug},
    )
    projects = data.get("projects", {}).get("nodes", [])
    if not projects:
        raise SystemExit(f"project not found: {project_slug}")
    project = projects[0]
    teams = project.get("teams", {}).get("nodes", [])
    team = next(
        (
            item
            for item in teams
            if item.get("name", "").lower() == team_name.lower()
            or item.get("key", "").lower() == team_name.lower()
        ),
        teams[0] if teams else None,
    )
    if not team:
        raise SystemExit(f"no team found for project {project_slug}")
    state = next(
        (item for item in team.get("states", {}).get("nodes", []) if item.get("name") == state_name),
        None,
    )
    if not state:
        raise SystemExit(f"state {state_name!r} not found for team {team.get('name')}")
    return {"project_id": project["id"], "team_id": team["id"], "state_id": state["id"]}


def create_issue(endpoint: str, api_key: str, targets: dict[str, str], title: str) -> dict[str, str]:
    timestamp = datetime.now().astimezone().isoformat(timespec="seconds")
    data = graphql(
        endpoint,
        api_key,
        """
        mutation ZhSmokeCreate($input: IssueCreateInput!) {
          issueCreate(input: $input) {
            success
            issue { id identifier url }
          }
        }
        """,
        {
            "input": {
                "teamId": targets["team_id"],
                "projectId": targets["project_id"],
                "stateId": targets["state_id"],
                "title": title,
                "description": ISSUE_TEMPLATE.format(timestamp=timestamp),
            }
        },
    )
    result = data.get("issueCreate", {})
    if not result.get("success"):
        raise SystemExit("Linear issueCreate returned success=false")
    issue = result["issue"]
    return {"id": issue["id"], "identifier": issue["identifier"], "url": issue["url"]}


def run_command(args: list[str]) -> None:
    print("+ " + " ".join(args), flush=True)
    subprocess.run(args, check=True)


def log_snapshot() -> set[Path]:
    return set(Path(".symphony/logs").glob("run-*.jsonl"))


def new_log_after(before: set[Path]) -> str:
    new_logs = sorted(log_snapshot() - before)
    if not new_logs:
        raise SystemExit("no new log found under .symphony/logs for this round")
    return str(new_logs[-1])


def main() -> None:
    args = parse_args()
    workflow_path = Path(args.workflow)
    api_key = os.environ.get("LINEAR_API_KEY") or workflow_value(workflow_path, "api_key")
    project_slug = workflow_value(workflow_path, "project_slug")
    endpoint = workflow_value(workflow_path, "endpoint") or "https://api.linear.app/graphql"
    if not api_key or not project_slug:
        raise SystemExit("workflow must provide tracker.api_key and tracker.project_slug, or set LINEAR_API_KEY")

    targets = resolve_targets(endpoint, api_key, project_slug, args.team, args.state)
    if args.dry_run:
        print(json.dumps({"project_slug": project_slug, "team": args.team, "state": args.state, "merge_target": args.merge_target or "(workflow default)", **targets}, ensure_ascii=False))
        return

    issue = create_issue(endpoint, api_key, targets, args.title)
    print(json.dumps(issue, ensure_ascii=False), flush=True)
    if args.create_only:
        return

    run_command(["make", "zh-smoke-stop"])
    before_logs = log_snapshot()
    make_args = ["make", "zh-smoke-once", f"ISSUE={issue['identifier']}"]
    if args.merge_target:
        make_args.append(f"MERGE_TARGET={args.merge_target}")
    run_command(make_args)
    log_path = new_log_after(before_logs)
    run_command(
        [
            "python3",
            "scripts/smoke_metrics.py",
            "--log",
            log_path,
            "--issue",
            issue["identifier"],
            "--append",
            args.results,
            "--markdown-append",
            args.markdown,
            "--change-note",
            args.change_note,
        ]
    )


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as exc:
        sys.exit(exc.returncode)
