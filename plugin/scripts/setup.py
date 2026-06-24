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

# --autonomous: granular permissions.allow (ADR §5.2) — the plugin's own MCP tools +
# safe read-only bash. Pipeline WRITE actions (build/commit/edits) are intentionally
# NOT auto-allowed; for fully hands-off runs the user sets permissions.defaultMode.
AUTONOMOUS_ALLOW = [
    "mcp__plugin_coding-agent_cks__*",
    "mcp__plugin_coding-agent_chainbench__*",
    "mcp__plugin_coding-agent_jira-gateway__*",
    "Bash(git status:*)", "Bash(git diff:*)", "Bash(git log:*)",
    "Bash(git rev-parse:*)", "Bash(git show:*)", "Bash(git branch:*)",
    "Bash(ls:*)", "Bash(cat:*)", "Bash(grep:*)", "Bash(rg:*)", "Bash(find:*)",
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


def _plugin_root() -> Path:
    """The installed plugin root (scripts/ lives directly under it)."""
    return Path(__file__).resolve().parent.parent


def _repo_root_env(plugin_root: Path, repo_root: Path, override: str | None) -> str | None:
    """Active domain pack's verification.repo_root_env name (e.g. GO_STABLENET_ROOT).

    project_id: --project override > single pack > repo-name match (mirrors doctor.py).
    """
    domains = plugin_root / "domains"
    packs = sorted(d.name for d in domains.glob("*")
                   if (d / "domain-pack.json").is_file()) if domains.is_dir() else []
    pid = override or (packs[0] if len(packs) == 1 else None)
    if not pid:
        base = repo_root.name
        pid = next((p for p in packs if p and p in base), None)
    if not pid:
        return None
    pack = _load_json(plugin_root / "domains" / pid / "domain-pack.json")
    return (pack.get("verification") or {}).get("repo_root_env")


def _merge_allow(path: Path, entries: list[str]) -> list[str]:
    """Merge entries into permissions.allow (dedup). Returns newly-added entries."""
    doc = _load_json(path)
    perms = doc.get("permissions") if isinstance(doc.get("permissions"), dict) else {}
    allow = perms.get("allow") if isinstance(perms.get("allow"), list) else []
    added = [e for e in entries if e not in allow]
    allow.extend(added)
    perms["allow"] = allow
    doc["permissions"] = perms
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(doc, indent=2) + "\n")
    return added


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
    ap.add_argument("--autonomous", action="store_true",
                    help="also register a granular permissions.allow (plugin MCP + read-only bash)")
    ap.add_argument("--project", default=None, help="domain pack project_id (else auto-detect)")
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
    rre = _repo_root_env(_plugin_root(), repo_root, args.project)

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
    is_plugin_repo = bool(repo_root) and (
        (repo_root / ".claude-plugin").is_dir() or (repo_root / "plugin" / ".claude-plugin").is_dir())
    pin_rre = bool(rre) and not (is_plugin_repo and not args.project)
    if rre and pin_rre:
        print(f"  {rre:<18} {'REPO-ROOT':<10} {repo_root}  [{PUBLIC}] (active pack repo_root_env)")
    elif rre:
        print(f"  {rre:<18} {'MISMATCH':<10} cwd is the coding-agent plugin repo, not a target "
              "project — repo_root_env NOT written (run from the target repo, or pass --project)")
    else:
        print(f"  {'repo_root_env':<18} {'UNKNOWN':<10} "
              "could not resolve active pack — pass --project <id>")
    print(f"  {'permissions':<18} {'NOTE':<10} "
          "--autonomous registers granular allow (MCP + read-only bash); for build/commit/edits "
          "also set permissions.defaultMode (not auto-set)")

    claude_dir = repo_root / ".claude"

    # --autonomous: register the allowlist independent of --fix / env resolution.
    if args.autonomous:
        w_allow = _merge_allow(claude_dir / "settings.local.json", AUTONOMOUS_ALLOW)
        _ensure_gitignored(repo_root, ".claude/settings.local.json")
        print(f"\nregistered {len(w_allow)} permission(s) to .claude/settings.local.json allow"
              + (f": {', '.join(w_allow)}" if w_allow else " (already present)"))

    if not args.fix:
        if missing:
            print(f"\n{len(missing)} value(s) unresolved: {', '.join(missing)}")
            print("run again with --fix (and --set KEY=VALUE for the missing ones, or --interactive)")
            return 1
        if not args.autonomous:
            print("\nall required values resolved. run with --fix to write them.")
        return 0

    # --fix: write resolved env into the two settings files (claude_dir defined above).
    public_vals = {k: resolved[k][0] for k, w, _ in REQUIRED if w == PUBLIC and resolved[k][0]}
    secret_vals = {k: resolved[k][0] for k, w, _ in REQUIRED if w == SECRET and resolved[k][0]}
    if pin_rre:
        public_vals[rre] = str(repo_root)   # pin active pack's repo_root_env to this repo

    w_pub = _merge_env(claude_dir / "settings.json", public_vals, args.force)
    w_sec = _merge_env(claude_dir / "settings.local.json", secret_vals, args.force)
    if w_sec:
        _ensure_gitignored(repo_root, ".claude/settings.local.json")

    print(f"\nwrote {len(w_pub)} key(s) to .claude/settings.json: {', '.join(w_pub) or '(none)'}")
    print(f"wrote {len(w_sec)} key(s) to .claude/settings.local.json: {', '.join(w_sec) or '(none)'}")
    if rre and not pin_rre:
        print(f"skipped {rre} (cwd is the plugin repo, not a target project)")
    if missing:
        print(f"still MISSING (provide via --set / --interactive): {', '.join(missing)}")
        return 1
    print("settings registered. restart the Claude Code session so the MCP servers pick up the env.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
