#!/usr/bin/env python3
"""setup.py — check and register the settings coding-agent needs to run.

The plugin's MCP servers (.mcp.json) read their binary paths and secrets from
${VAR} substitutions, which Claude Code resolves from the session env. This
script makes those values present in the *project* settings so a freshly
installed plugin "just works":

  - public/path values  -> {repo_root}/.claude/settings.json        ("env" block)
  - secrets (API token) -> {repo_root}/.claude/settings.local.json  ("env" block)

Values are filled in this order: explicit --set, existing process env, then
auto-detection of sibling repos. Anything still missing is reported (use --fix
with --set, or --interactive to be prompted). settings.local.json is added to
.gitignore so the token is never committed.

Modes:
  --check        report status, exit 1 if any REQUIRED value is unresolved (default)
  --fix          write resolved values into the settings files (idempotent merge)
  --set K=V      provide a value explicitly (repeatable); wins over detection
  --interactive  prompt on stdin for any value still missing (human terminal use)
  --force        overwrite values already present in settings (default: keep them)

Stdlib only. Run from inside the project where you use coding-agent.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

# (key, where, required, how-to-find)
PUBLIC = "settings.json"
SECRET = "settings.local.json"

REQUIRED = [
    ("CKS_MCP_BIN", PUBLIC, "cks knowledge-search MCP binary"),
    ("CKS_CONFIG", PUBLIC, "cks config yaml (ckg/ckv paths)"),
    ("JIRA_GATEWAY_BIN", PUBLIC, "jira-gateway MCP binary"),
    ("JIRA_BASE_URL", PUBLIC, "Jira site URL, e.g. https://your.atlassian.net"),
    ("JIRA_USER_EMAIL", PUBLIC, "Jira account email"),
    ("CHAINBENCH_DIR", PUBLIC, "chainbench checkout directory"),
    ("JIRA_API_TOKEN", SECRET, "Jira API token (secret)"),
]


def _repo_root() -> Path:
    try:
        out = subprocess.run(["git", "rev-parse", "--show-toplevel"],
                             capture_output=True, text=True, check=True)
        return Path(out.stdout.strip())
    except (subprocess.CalledProcessError, FileNotFoundError):
        return Path.cwd()


def _first_existing(*paths: Path) -> str | None:
    for p in paths:
        if p and p.exists():
            return str(p)
    return None


def _detect(repo_root: Path) -> dict[str, str]:
    """Best-effort auto-detection of path-style values from sibling repos."""
    base = repo_root.parent  # e.g. .../github/, sibling to code-knowledge-system
    cks = base / "code-knowledge-system"
    ca = base / "coding-agent"
    found: dict[str, str] = {}

    v = _first_existing(cks / "bin" / "cks-mcp")
    if v:
        found["CKS_MCP_BIN"] = v
    # cks config: prefer a stablenet-scoped config, else a generic cks.yaml
    if cks.is_dir():
        for name in ("cks-stablenet.yaml", "cks.yaml"):
            v = _first_existing(cks / name)
            if v:
                found["CKS_CONFIG"] = v
                break
    v = _first_existing(ca / "tools" / "jira-gateway-mcp" / "bin" / "jira-gateway-mcp",
                        repo_root / "tools" / "jira-gateway-mcp" / "bin" / "jira-gateway-mcp")
    if v:
        found["JIRA_GATEWAY_BIN"] = v
    v = _first_existing(base / "chainbench")
    if v:
        found["CHAINBENCH_DIR"] = v
    return found


def _resolve(key: str, overrides: dict[str, str], detected: dict[str, str]) -> tuple[str | None, str]:
    """Return (value, source). source in {set, env, detected, none}."""
    if key in overrides:
        return overrides[key], "set"
    if os.environ.get(key):
        return os.environ[key], "env"
    if key in detected:
        return detected[key], "detected"
    return None, "none"


def _load_json(path: Path) -> dict:
    try:
        return json.loads(path.read_text())
    except (OSError, json.JSONDecodeError):
        return {}


def _merge_env(path: Path, values: dict[str, str], force: bool) -> list[str]:
    """Merge values into the "env" block of a settings file. Returns keys written."""
    doc = _load_json(path)
    env = doc.get("env")
    if not isinstance(env, dict):
        env = {}
    written = []
    for k, v in values.items():
        if not v:
            continue
        if env.get(k) and not force:
            continue  # keep existing
        env[k] = v
        written.append(k)
    doc["env"] = env
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(doc, indent=2) + "\n")
    return written


def _ensure_gitignored(repo_root: Path, rel: str) -> None:
    gi = repo_root / ".gitignore"
    line = rel
    existing = gi.read_text().splitlines() if gi.is_file() else []
    if line in existing:
        return
    with gi.open("a") as fh:
        if existing and existing[-1] != "":
            fh.write("\n")
        fh.write(f"# coding-agent local secrets\n{line}\n")


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description="Check/register coding-agent settings")
    ap.add_argument("--check", action="store_true", help="report only (default)")
    ap.add_argument("--fix", action="store_true", help="write resolved values")
    ap.add_argument("--set", action="append", default=[], metavar="KEY=VALUE",
                    help="explicit value (repeatable), wins over detection")
    ap.add_argument("--interactive", action="store_true", help="prompt for missing values")
    ap.add_argument("--force", action="store_true", help="overwrite existing settings values")
    args = ap.parse_args(argv)

    overrides: dict[str, str] = {}
    for item in args.set:
        if "=" not in item:
            print(f"error: --set expects KEY=VALUE, got {item!r}", file=sys.stderr)
            return 2
        k, v = item.split("=", 1)
        overrides[k.strip()] = v.strip()

    repo_root = _repo_root()
    detected = _detect(repo_root)

    resolved: dict[str, tuple[str | None, str]] = {}
    for key, _where, _desc in REQUIRED:
        resolved[key] = _resolve(key, overrides, detected)

    # Interactive fallback for anything still unresolved.
    if args.interactive:
        for key, _where, desc in REQUIRED:
            if resolved[key][0] is None:
                try:
                    ans = input(f"{key} ({desc}): ").strip()
                except EOFError:
                    ans = ""
                if ans:
                    resolved[key] = (ans, "set")

    # Report.
    chainbench_mcp = shutil.which("chainbench-mcp")
    print(f"coding-agent setup — project: {repo_root}")
    print(f"  {'KEY':<18} {'STATUS':<10} SOURCE / VALUE")
    missing = []
    for key, where, _desc in REQUIRED:
        val, src = resolved[key]
        if val is None:
            missing.append(key)
            print(f"  {key:<18} {'MISSING':<10} -> needs --set {key}=... or --interactive")
        else:
            shown = "********" if where == SECRET else val
            print(f"  {key:<18} {src.upper():<10} {shown}  [{where}]")
    print(f"  {'chainbench-mcp':<18} {'OK' if chainbench_mcp else 'NOT ON PATH':<10} "
          f"{chainbench_mcp or '-> install chainbench so chainbench-mcp is on PATH'}")
    print(f"  {'permissions':<18} {'NOTE':<10} "
          "for hands-off runs set permissions.defaultMode=bypassPermissions (not auto-set)")

    if not args.fix:
        if missing:
            print(f"\n{len(missing)} value(s) unresolved: {', '.join(missing)}")
            print("run again with --fix (and --set KEY=VALUE for the missing ones, or --interactive)")
            return 1
        print("\nall required values resolved. run with --fix to write them.")
        return 0

    # --fix: write resolved values into the two settings files.
    public_vals = {k: resolved[k][0] for k, w, _ in REQUIRED if w == PUBLIC and resolved[k][0]}
    secret_vals = {k: resolved[k][0] for k, w, _ in REQUIRED if w == SECRET and resolved[k][0]}

    claude_dir = repo_root / ".claude"
    w_pub = _merge_env(claude_dir / "settings.json", public_vals, args.force)
    w_sec = _merge_env(claude_dir / "settings.local.json", secret_vals, args.force)
    if w_sec:
        _ensure_gitignored(repo_root, ".claude/settings.local.json")

    print(f"\nwrote {len(w_pub)} key(s) to .claude/settings.json: {', '.join(w_pub) or '(none)'}")
    print(f"wrote {len(w_sec)} key(s) to .claude/settings.local.json: {', '.join(w_sec) or '(none)'}")
    if missing:
        print(f"still MISSING (provide via --set / --interactive): {', '.join(missing)}")
        return 1
    print("settings registered. restart the Claude Code session so the MCP servers pick up the env.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
