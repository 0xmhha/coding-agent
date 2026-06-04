"""Aggregate RunResults into a 3-way (A/B/C) comparison report.

Emits JSON (machine), Markdown (human), and CSV (per-cell). Generalizes the
2-way vanilla-vs-omc shape of oh-my-claudecode benchmark/compare_results.py to N
modes, and adds the cost column its report leaves at zero.

Stdlib only.
"""

from __future__ import annotations

import csv
import io
import json

from .collect import RunResult

REPORT_SCHEMA_VERSION = 1


def aggregate(results: list[RunResult]) -> dict:
    """Per-mode rollup: counts, correctness rate, and averaged metrics."""
    modes: dict[str, dict] = {}
    for r in results:
        m = modes.setdefault(
            r.mode,
            {"mode": r.mode, "cells": 0, "evaluated": 0, "correct": 0,
             "tokens_sum": 0, "cost_sum": 0.0, "latency_sum": 0.0, "latency_n": 0,
             "safety_flags": 0, "cost_status": set()},
        )
        m["cells"] += 1
        m["tokens_sum"] += r.usage.total()
        m["cost_sum"] += r.cost_usd
        m["cost_status"].add(r.cost_status)
        m["safety_flags"] += len(r.safety_flags)
        if r.correct is not None:
            m["evaluated"] += 1
            if r.correct:
                m["correct"] += 1
        if r.latency_s is not None:
            m["latency_sum"] += r.latency_s
            m["latency_n"] += 1

    rollups: dict[str, dict] = {}
    for mode, m in modes.items():
        cells = m["cells"] or 1
        statuses = m["cost_status"]
        cost_status = (
            "actual" if statuses == {"actual"}
            else "estimated" if "estimated" in statuses
            else "unknown" if statuses <= {"unknown"} else "mixed"
        )
        rollups[mode] = {
            "mode": mode,
            "cells": m["cells"],
            "correct": m["correct"],
            "evaluated": m["evaluated"],
            "correct_rate": round(m["correct"] / m["evaluated"], 4) if m["evaluated"] else None,
            "avg_tokens": round(m["tokens_sum"] / cells, 1),
            "avg_cost_usd": round(m["cost_sum"] / cells, 6),
            "avg_latency_s": round(m["latency_sum"] / m["latency_n"], 2) if m["latency_n"] else None,
            "safety_flags": m["safety_flags"],
            "cost_status": cost_status,
        }
    return rollups


def build_report(results: list[RunResult], experiment: str = "", generated_at: str = "") -> dict:
    rollups = aggregate(results)
    # Per-task rows: {task: {mode: {correct, tokens, cost, latency}}}
    tasks: dict[str, dict] = {}
    for r in results:
        tasks.setdefault(r.task, {})[r.mode] = {
            "correct": r.correct,
            "tokens": r.usage.total(),
            "cost_usd": r.cost_usd,
            "latency_s": r.latency_s,
            "safety_flags": r.safety_flags,
            "pipeline_state": r.pipeline_state,
        }
    return {
        "schema_version": REPORT_SCHEMA_VERSION,
        "experiment": experiment,
        "generated_at": generated_at,
        "modes": rollups,
        "tasks": tasks,
        "cells": [r.as_dict() for r in results],
    }


def _fmt(v, dash="—"):
    return dash if v is None else v


def to_markdown(report: dict) -> str:
    modes = report.get("modes", {})
    order = sorted(modes.keys())  # deterministic; A_/B_/C_ sort naturally
    lines: list[str] = []
    lines.append(f"# Bench comparison — {report.get('experiment', '')}".rstrip())
    if report.get("generated_at"):
        lines.append(f"_generated: {report['generated_at']}_")
    lines.append("")
    lines.append("## Mode rollup")
    lines.append("")
    lines.append("| mode | cells | correct | avg_tokens | avg_cost($) | avg_latency(s) | safety_flags | cost |")
    lines.append("|------|-------|---------|------------|-------------|----------------|--------------|------|")
    for mode in order:
        m = modes[mode]
        correct = f"{m['correct']}/{m['evaluated']}" if m["evaluated"] else "—"
        lines.append(
            f"| {mode} | {m['cells']} | {correct} | {m['avg_tokens']} | "
            f"{m['avg_cost_usd']} | {_fmt(m['avg_latency_s'])} | {m['safety_flags']} | {m['cost_status']} |"
        )
    lines.append("")

    # Per-task A/B/C correctness + tokens + cost.
    tasks = report.get("tasks", {})
    if tasks:
        lines.append("## Per-task (correct · tokens · $)")
        lines.append("")
        header = "| task | " + " | ".join(order) + " |"
        sep = "|------|" + "|".join(["------"] * len(order)) + "|"
        lines.append(header)
        lines.append(sep)
        for task in sorted(tasks):
            cells = tasks[task]
            row = [task]
            for mode in order:
                c = cells.get(mode)
                if not c:
                    row.append("—")
                    continue
                mark = "✓" if c["correct"] else ("✗" if c["correct"] is False else "?")
                row.append(f"{mark} · {c['tokens']} · ${c['cost_usd']}")
            lines.append("| " + " | ".join(row) + " |")
        lines.append("")

    if any(m.get("cost_status") in ("estimated", "mixed") for m in modes.values()):
        lines.append("> cost marked `estimated` is derived from transcript char counts "
                     "(chars/4); supply real Claude session JSONL for `actual` cost.")
        lines.append("")
    return "\n".join(lines)


def to_csv(results: list[RunResult]) -> str:
    buf = io.StringIO()
    w = csv.writer(buf)
    w.writerow(["task", "mode", "status", "pipeline_state", "correct",
                "tokens", "cost_usd", "cost_status", "usage_source",
                "latency_s", "safety_flags"])
    for r in results:
        w.writerow([
            r.task, r.mode, r.status, r.pipeline_state,
            "" if r.correct is None else int(r.correct),
            r.usage.total(), r.cost_usd, r.cost_status, r.usage_source,
            "" if r.latency_s is None else r.latency_s,
            ";".join(r.safety_flags),
        ])
    return buf.getvalue()
