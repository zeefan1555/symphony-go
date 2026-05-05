#!/usr/bin/env python3
"""Extract fixed smoke workflow metrics from Symphony JSONL logs."""

from __future__ import annotations

import argparse
import csv
import json
import re
from dataclasses import dataclass, fields
from datetime import datetime
from pathlib import Path
from typing import Any


@dataclass
class Metrics:
    issue: str
    log: str
    session_started_at: str = ""
    turn_started_at: str = ""
    turn_completed_at: str = ""
    todo_to_progress_at: str = ""
    progress_to_review_at: str = ""
    progress_to_ai_review_at: str = ""
    ai_review_to_rework_at: str = ""
    ai_review_to_merging_at: str = ""
    review_to_merging_at: str = ""
    merging_to_done_at: str = ""
    merge_duration_ms: str = ""
    turn_duration_ms: str = ""
    input_tokens: str = ""
    output_tokens: str = ""
    total_tokens: str = ""
    last_input_tokens: str = ""
    last_output_tokens: str = ""
    last_total_tokens: str = ""
    token_events: str = ""
    total_flow_ms: str = ""
    dispatch_latency_ms: str = ""
    post_turn_handoff_ms: str = ""
    ai_review_ms: str = ""
    merge_state_ms: str = ""
    state_change_count: str = ""
    state_flow: str = ""
    command_events: str = ""
    plan_updates: str = ""
    agent_messages: str = ""
    final_messages: str = ""
    diff_updates: str = ""
    file_changes: str = ""
    workpad_updates: str = ""
    workpad_phases: str = ""
    workpad_bytes: str = ""
    workpad_lines: str = ""
    final_state: str = ""
    commit: str = ""
    changed_files: str = ""
    ai_review_result: str = ""
    success: str = "false"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Extract token and speed metrics for the Chinese smoke benchmark."
    )
    parser.add_argument("--log", help="JSONL log path. Defaults to latest .symphony/logs/run-*.jsonl")
    parser.add_argument("--issue", help="Issue identifier to filter, for example ZEE-16")
    parser.add_argument("--append", help="Append TSV output to this file")
    parser.add_argument("--markdown-append", help="Append a Markdown experiment row to this file")
    parser.add_argument("--change-note", default="", help="Human-readable note describing the optimization in this round")
    return parser.parse_args()


def latest_log() -> Path:
    logs = sorted(Path(".symphony/logs").glob("run-*.jsonl"))
    if not logs:
        raise SystemExit("no logs found under .symphony/logs")
    return logs[-1]


def load_events(path: Path) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    with path.open(encoding="utf-8") as fh:
        for line_no, line in enumerate(fh, 1):
            line = line.strip()
            if not line:
                continue
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError as exc:
                raise SystemExit(f"{path}:{line_no}: invalid JSON: {exc}") from exc
    return events


def event_issue(event: dict[str, Any]) -> str:
    fields_map = event.get("fields") if isinstance(event.get("fields"), dict) else {}
    return (
        str(event.get("issue_identifier") or "")
        or str(event.get("issue") or "")
        or str(fields_map.get("issue_identifier") or "")
    )


def token_usage(event: dict[str, Any]) -> tuple[int, int, int, int, int, int] | None:
    fields_map = event.get("fields") if isinstance(event.get("fields"), dict) else {}
    params = fields_map.get("params") if isinstance(fields_map.get("params"), dict) else {}
    usage = params.get("tokenUsage") if isinstance(params.get("tokenUsage"), dict) else params
    total = usage.get("total") if isinstance(usage.get("total"), dict) else usage
    last = usage.get("last") if isinstance(usage.get("last"), dict) else total
    if not isinstance(total, dict):
        return None
    input_tokens = int(total.get("inputTokens") or total.get("input_tokens") or 0)
    output_tokens = int(total.get("outputTokens") or total.get("output_tokens") or 0)
    total_tokens = int(total.get("totalTokens") or total.get("total_tokens") or 0)
    last_input_tokens = int(last.get("inputTokens") or last.get("input_tokens") or 0)
    last_output_tokens = int(last.get("outputTokens") or last.get("output_tokens") or 0)
    last_total_tokens = int(last.get("totalTokens") or last.get("total_tokens") or 0)
    if (
        input_tokens == 0
        and output_tokens == 0
        and total_tokens == 0
        and last_input_tokens == 0
        and last_output_tokens == 0
        and last_total_tokens == 0
    ):
        return None
    if total_tokens == 0:
        total_tokens = input_tokens + output_tokens
    if last_total_tokens == 0:
        last_total_tokens = last_input_tokens + last_output_tokens
    return input_tokens, output_tokens, total_tokens, last_input_tokens, last_output_tokens, last_total_tokens


def turn_duration(event: dict[str, Any]) -> int | None:
    fields_map = event.get("fields") if isinstance(event.get("fields"), dict) else {}
    params = fields_map.get("params") if isinstance(fields_map.get("params"), dict) else {}
    turn = params.get("turn") if isinstance(params.get("turn"), dict) else {}
    duration = turn.get("durationMs")
    return int(duration) if isinstance(duration, (int, float)) else None


def seconds_between(start: str, end: str) -> str:
    if not start or not end:
        return ""
    try:
        start_dt = parse_timestamp(start)
        end_dt = parse_timestamp(end)
    except ValueError:
        return ""
    return f"{(end_dt - start_dt).total_seconds():.3f}"


def parse_timestamp(value: str) -> datetime:
    value = value.strip()
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    try:
        return datetime.fromisoformat(value)
    except ValueError:
        match = re.match(r"^(.+T\d\d:\d\d:\d\d)\.(\d+)([+-]\d\d:\d\d)$", value)
        if not match:
            raise
        base, fraction, zone = match.groups()
        normalized = (fraction + "000000")[:6]
        return datetime.fromisoformat(f"{base}.{normalized}{zone}")


def millis_between(start: str, end: str) -> str:
    seconds = seconds_between(start, end)
    if not seconds:
        return ""
    return str(int(float(seconds) * 1000))


def increment(value: str) -> str:
    return str(int(value or "0") + 1)


def add_int(value: str, amount: int) -> str:
    return str(int(value or "0") + amount)


def item_type(event: dict[str, Any]) -> str:
    fields_map = event.get("fields") if isinstance(event.get("fields"), dict) else {}
    params = fields_map.get("params") if isinstance(fields_map.get("params"), dict) else {}
    item = params.get("item") if isinstance(params.get("item"), dict) else {}
    return str(item.get("type") or "")


def item_phase(event: dict[str, Any]) -> str:
    fields_map = event.get("fields") if isinstance(event.get("fields"), dict) else {}
    params = fields_map.get("params") if isinstance(fields_map.get("params"), dict) else {}
    item = params.get("item") if isinstance(params.get("item"), dict) else {}
    return str(item.get("phase") or "")


def extract_metrics(events: list[dict[str, Any]], log_path: Path, issue: str | None) -> Metrics:
    selected_issue = issue or first_issue(events)
    if not selected_issue:
        raise SystemExit("could not infer issue; pass --issue")

    metrics = Metrics(issue=selected_issue, log=str(log_path))
    state_flow: list[str] = []
    workpad_phases: list[str] = []
    for event in events:
        if event_issue(event) != selected_issue:
            continue

        timestamp = str(event.get("time") or "")
        name = str(event.get("event") or "")
        message = str(event.get("message") or "")

        if name == "state_changed":
            metrics.state_change_count = increment(metrics.state_change_count)
            state_flow.append(message.replace(" -> ", "->"))
            if message == "Todo -> In Progress":
                metrics.todo_to_progress_at = timestamp
                metrics.final_state = "In Progress"
            elif message == "In Progress -> Human Review":
                metrics.progress_to_review_at = timestamp
                metrics.final_state = "Human Review"
                fields_map = event.get("fields") if isinstance(event.get("fields"), dict) else {}
                metrics.commit = str(fields_map.get("commit") or "")
                changed = fields_map.get("changed_files")
                if isinstance(changed, list):
                    metrics.changed_files = ",".join(str(item) for item in changed)
            elif message == "In Progress -> AI Review":
                metrics.progress_to_ai_review_at = timestamp
                metrics.final_state = "AI Review"
                fields_map = event.get("fields") if isinstance(event.get("fields"), dict) else {}
                metrics.commit = str(fields_map.get("commit") or metrics.commit)
                changed = fields_map.get("changed_files")
                if isinstance(changed, list):
                    metrics.changed_files = ",".join(str(item) for item in changed)
            elif message == "AI Review -> Rework":
                metrics.ai_review_to_rework_at = timestamp
                metrics.final_state = "Rework"
            elif message == "AI Review -> Merging":
                metrics.ai_review_to_merging_at = timestamp
                metrics.final_state = "Merging"
            elif message == "Human Review -> Merging":
                metrics.review_to_merging_at = timestamp
                metrics.final_state = "Merging"
            elif message == "Merging -> Done":
                metrics.merging_to_done_at = timestamp
                metrics.final_state = "Done"

        if name == "codex_event" and message == "session_started":
            metrics.session_started_at = timestamp
        elif name == "codex_event" and message == "turn_started":
            metrics.turn_started_at = timestamp
        elif name == "codex_event" and message == "turn/completed":
            metrics.turn_completed_at = timestamp
            duration = turn_duration(event)
            if duration is not None:
                metrics.turn_duration_ms = str(duration)
        elif name == "codex_event" and message == "thread/tokenUsage/updated":
            usage = token_usage(event)
            if usage is not None:
                metrics.input_tokens, metrics.output_tokens, metrics.total_tokens = map(str, usage[:3])
                metrics.last_input_tokens, metrics.last_output_tokens, metrics.last_total_tokens = map(str, usage[3:])
                metrics.token_events = increment(metrics.token_events)
        elif name == "codex_event" and message == "turn/plan/updated":
            metrics.plan_updates = increment(metrics.plan_updates)
        elif name == "codex_event" and message == "turn/diff/updated":
            metrics.diff_updates = increment(metrics.diff_updates)
        elif name == "codex_event" and message == "item/completed":
            typ = item_type(event)
            if typ == "commandExecution":
                metrics.command_events = increment(metrics.command_events)
            elif typ == "fileChange":
                metrics.file_changes = increment(metrics.file_changes)
            elif typ == "agentMessage":
                if item_phase(event) == "final_answer":
                    metrics.final_messages = increment(metrics.final_messages)
                else:
                    metrics.agent_messages = increment(metrics.agent_messages)
        elif name == "ai_review_completed":
            metrics.ai_review_result = "passed"
        elif name == "ai_review_failed":
            metrics.ai_review_result = "failed"
        elif name == "workpad_updated":
            fields_map = event.get("fields") if isinstance(event.get("fields"), dict) else {}
            metrics.workpad_updates = increment(metrics.workpad_updates)
            phase = str(fields_map.get("phase") or "")
            if phase:
                workpad_phases.append(phase)
            if isinstance(fields_map.get("bytes"), int):
                metrics.workpad_bytes = add_int(metrics.workpad_bytes, int(fields_map["bytes"]))
            if isinstance(fields_map.get("lines"), int):
                metrics.workpad_lines = add_int(metrics.workpad_lines, int(fields_map["lines"]))
        elif name == "local_merge_completed":
            fields_map = event.get("fields") if isinstance(event.get("fields"), dict) else {}
            duration = fields_map.get("duration_ms")
            if isinstance(duration, (int, float)):
                metrics.merge_duration_ms = str(int(duration))

    if not metrics.turn_duration_ms:
        seconds = seconds_between(metrics.turn_started_at, metrics.turn_completed_at)
        if seconds:
            metrics.turn_duration_ms = str(int(float(seconds) * 1000))

    metrics.total_flow_ms = millis_between(metrics.todo_to_progress_at, metrics.merging_to_done_at)
    metrics.dispatch_latency_ms = millis_between(metrics.todo_to_progress_at, metrics.turn_started_at)
    review_at = metrics.progress_to_ai_review_at or metrics.progress_to_review_at
    metrics.post_turn_handoff_ms = millis_between(metrics.turn_completed_at, review_at)
    review_done_at = metrics.ai_review_to_merging_at or metrics.ai_review_to_rework_at
    metrics.ai_review_ms = millis_between(metrics.progress_to_ai_review_at, review_done_at)
    merge_started_at = metrics.ai_review_to_merging_at or metrics.review_to_merging_at
    metrics.merge_state_ms = millis_between(merge_started_at, metrics.merging_to_done_at)
    metrics.state_flow = " > ".join(state_flow)
    metrics.workpad_phases = ",".join(workpad_phases)
    metrics.success = str(metrics.final_state == "Done" and bool(metrics.commit)).lower()
    return metrics


def first_issue(events: list[dict[str, Any]]) -> str:
    for event in events:
        issue = event_issue(event)
        if issue:
            return issue
    return ""


def write_tsv(metrics: Metrics, append_path: str | None) -> None:
    names = [field.name for field in fields(Metrics)]
    row = {name: getattr(metrics, name) for name in names}

    if append_path:
        path = Path(append_path)
        path.parent.mkdir(parents=True, exist_ok=True)
        upsert_tsv_row(path, names, row)

    writer = csv.DictWriter(__import__("sys").stdout, fieldnames=names, delimiter="\t", lineterminator="\n")
    writer.writeheader()
    writer.writerow(row)


def upsert_tsv_row(path: Path, names: list[str], row: dict[str, str]) -> None:
    rows: list[dict[str, str]] = []
    if path.exists() and path.stat().st_size > 0:
        with path.open(encoding="utf-8", newline="") as fh:
            reader = csv.DictReader(fh, delimiter="\t")
            rows = [{name: existing.get(name, "") for name in names} for existing in reader]
    replaced = False
    for index, existing in enumerate(rows):
        if existing.get("issue") == row.get("issue") and existing.get("log") == row.get("log"):
            rows[index] = row
            replaced = True
            break
    if not replaced:
        rows.append(row)
    with path.open("w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=names, delimiter="\t", lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)


def append_markdown(metrics: Metrics, markdown_path: str | None, change_note: str = "") -> None:
    if not markdown_path:
        return
    path = Path(markdown_path)
    marker = "## 实验记录"
    header = (
        "| 时间 | Issue | 优化动作 | 状态 | 成功 | Token | Events | Flow ms | Turn ms | AI Review | Merge ms | Workpad | 文件 | 日志 |\n"
        "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n"
    )
    now = datetime.now().isoformat(timespec="seconds")
    note = change_note.strip() or "未填写"
    events = (
        f"tok {metrics.token_events}, cmd {metrics.command_events}, "
        f"plan {metrics.plan_updates or '0'}, diff {metrics.diff_updates or '0'}"
    )
    workpad = metrics.workpad_updates or "0"
    if metrics.workpad_phases:
        workpad += f" ({metrics.workpad_phases})"
    row = (
        f"| {now} | {metrics.issue} | {note} | {metrics.final_state} | {metrics.success} | "
        f"{metrics.total_tokens} (last {metrics.last_total_tokens}, events {metrics.token_events}) | "
        f"{events} | {metrics.total_flow_ms} | {metrics.turn_duration_ms} | {metrics.ai_review_result} | "
        f"{metrics.merge_duration_ms} | {workpad} | {metrics.changed_files} | `{metrics.log}` |\n"
    )
    if not path.exists():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(f"{marker}\n\n{header}{row}", encoding="utf-8")
        return
    content = path.read_text(encoding="utf-8")
    if marker not in content:
        suffix = "" if content.endswith("\n") else "\n"
        path.write_text(content + suffix + f"\n{marker}\n\n{header}{row}", encoding="utf-8")
        return
    if header.strip() not in content:
        path.write_text(content.rstrip() + f"\n\n{header}{row}", encoding="utf-8")
        return
    path.write_text(upsert_markdown_table_row(content, header, row, metrics), encoding="utf-8")


def upsert_markdown_table_row(content: str, header: str, row: str, metrics: Metrics) -> str:
    lines = [line for line in content.splitlines(keepends=True) if not markdown_row_matches(line, metrics)]
    header_lines = header.splitlines(keepends=True)
    header_index = find_line_sequence(lines, header_lines)
    if header_index < 0:
        suffix = "" if content.endswith("\n") else "\n"
        return content + suffix + row

    insert_at = header_index + len(header_lines)
    while insert_at < len(lines) and lines[insert_at].startswith("| "):
        insert_at += 1
    lines.insert(insert_at, row)
    return "".join(lines)


def find_line_sequence(lines: list[str], sequence: list[str]) -> int:
    if not sequence:
        return -1
    for index in range(0, len(lines) - len(sequence) + 1):
        if lines[index : index + len(sequence)] == sequence:
            return index
    return -1


def markdown_row_matches(line: str, metrics: Metrics) -> bool:
    return (
        line.startswith("| ")
        and f"| {metrics.issue} |" in line
        and f"`{metrics.log}`" in line
    )


def main() -> None:
    args = parse_args()
    log_path = Path(args.log) if args.log else latest_log()
    metrics = extract_metrics(load_events(log_path), log_path, args.issue)
    write_tsv(metrics, args.append)
    append_markdown(metrics, args.markdown_append, args.change_note)


if __name__ == "__main__":
    main()
