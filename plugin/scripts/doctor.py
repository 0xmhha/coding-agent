#!/usr/bin/env python3
"""doctor.py — read-only environment diagnostics for the coding-agent plugin.

Deterministic checks only (no LLM, no MCP): active plugin version, project/repo,
active domain pack + project_id, env vars (process env + .claude/settings*.json,
secrets masked, restart-needed detection), cks config source_root, and
permissions/allowlist. The `/coding-agent:doctor` command runs this, then adds the
LIVE MCP health probes (cks/chainbench/jira) and the final verdict.

    python3 doctor.py --plugin-root "$CLAUDE_PLUGIN_ROOT" [--project-id ID] [--json]

Run from the target project root (so `git rev-parse` resolves the repo). Stdlib
only; PyYAML is used opportunistically to read the cks config's source_root.
"""
from __future__ import annotations

import argparse
import json
import os
import subprocess
from pathlib import Path

SECRETS = {"JIRA_API_TOKEN"}
ENV_KEYS = ["CKS_CONFIG", "CKS_MCP_BIN", "CHAINBENCH_DIR",
            "JIRA_BASE_URL", "JIRA_USER_EMAIL", "JIRA_API_TOKEN"]


def _run(cmd: list[str]) -> str:
    try:
        return subprocess.run(cmd, capture_output=True, text=True).stdout.strip()
    except Exception:
        return ""


def _load_json(p: Path) -> dict:
    try:
        return json.loads(p.read_text())
    except Exception:
        return {}


def _mask(key: str, val):
    return "********" if (key in SECRETS and val) else val


def detect_project_id(plugin_root: Path, repo_root: str, override):
    """Resolve project_id: --project-id > single pack > repo-name/remote match > None."""
    domains = plugin_root / "domains"
    packs = sorted(d.name for d in domains.glob("*")
                   if (d / "domain-pack.json").is_file()) if domains.is_dir() else []
    if override:
        return override, packs
    if len(packs) == 1:
        return packs[0], packs
    base = Path(repo_root).name if repo_root else ""
    remote = _run(["git", "-C", repo_root or ".", "remote", "get-url", "origin"])
    for pid in packs:
        if pid and (pid in base or pid in remote):
            return pid, packs
    return None, packs


def diagnose(plugin_root: Path | None, project_id_override) -> dict:
    out: dict = {"issues": [], "restart_needed": []}

    # --- plugin ---
    ver = "?"
    if plugin_root:
        ver = _load_json(plugin_root / ".claude-plugin" / "plugin.json").get("version", "?")
    out["plugin"] = {"active_version": ver, "plugin_root": str(plugin_root) if plugin_root else None}
    if not plugin_root:
        out["issues"].append("CLAUDE_PLUGIN_ROOT not provided (pass --plugin-root); cannot read active version/packs")

    # --- project / repo ---
    repo_root = _run(["git", "rev-parse", "--show-toplevel"])
    head = _run(["git", "rev-parse", "--short", "HEAD"])
    out["project"] = {"cwd": os.getcwd(), "is_git": bool(repo_root),
                      "repo_root": repo_root or None, "head": head or None}
    if not repo_root:
        out["issues"].append("not inside a git repo — run /coding-agent:doctor from the target project root")

    # --- domain pack / project_id ---
    pid, packs, repo_root_env = None, [], None
    if plugin_root:
        pid, packs = detect_project_id(plugin_root, repo_root, project_id_override)
        if pid:
            pack = _load_json(plugin_root / "domains" / pid / "domain-pack.json")
            repo_root_env = (pack.get("verification") or {}).get("repo_root_env")
        else:
            out["issues"].append(f"could not resolve project_id (packs={packs}); pass --project <id>")
    out["domain_pack"] = {"project_id": pid, "available_packs": packs, "repo_root_env": repo_root_env}

    # --- env vars (process + settings) ---
    settings = _load_json(Path(repo_root) / ".claude" / "settings.json") if repo_root else {}
    slocal = _load_json(Path(repo_root) / ".claude" / "settings.local.json") if repo_root else {}
    senv = {**settings.get("env", {}), **slocal.get("env", {})}
    keys = ([repo_root_env] if repo_root_env else []) + ENV_KEYS
    env_report = {}
    seen = set()
    for k in keys:
        if not k or k in seen:
            continue
        seen.add(k)
        pv, sv = os.environ.get(k), senv.get(k)
        status = "ok" if pv else ("restart_needed" if sv else "unset")
        env_report[k] = {"process": _mask(k, pv), "settings": _mask(k, sv), "status": status}
        if sv and not pv:
            out["restart_needed"].append(k)
        elif not pv and not sv and k == repo_root_env:
            out["issues"].append(f"{k} unset — repo_root falls back to git rev-parse; `setup --fix` can pin it")
    out["env"] = env_report

    # --- cks config (presence only) ---
    # source_root / indexed_head coherence is probed LIVE by the command via
    # cks_ops_health (authoritative); the yaml schema nests it under backends and
    # varies, so we do not parse it here.
    cks_cfg = os.environ.get("CKS_CONFIG") or senv.get("CKS_CONFIG")
    cks = {"path": cks_cfg, "exists": bool(cks_cfg and Path(cks_cfg).is_file()),
           "note": "source_root / freshness checked live by the command (cks_ops_health)"}
    if cks_cfg and not Path(cks_cfg).is_file():
        out["issues"].append(f"CKS_CONFIG points to a missing file: {cks_cfg}")
    out["cks_config"] = cks

    # --- permissions / allowlist ---
    perms = settings.get("permissions", {})
    allow = perms.get("allow", []) or []
    out["permissions"] = {"defaultMode": perms.get("defaultMode"),
                          "plugin_allowlisted": any("coding-agent" in str(x) for x in allow),
                          "allow_count": len(allow)}

    out["verdict"] = "READY" if not out["issues"] and not out["restart_needed"] else "ATTENTION"
    return out


def render(out: dict) -> str:
    L = [f"coding-agent doctor — {out['verdict']}  (deterministic; MCP probes added by the command)"]
    p = out["plugin"]; L.append(f"  plugin     : v{p['active_version']}  ({p['plugin_root']})")
    pr = out["project"]; L.append(f"  project    : {pr['repo_root'] or pr['cwd']} @ {pr['head'] or '-'} (git={pr['is_git']})")
    dp = out["domain_pack"]
    L.append(f"  domain pack: project_id={dp['project_id']} packs={dp['available_packs']} repo_root_env={dp['repo_root_env']}")
    L.append("  env:")
    for k, e in out["env"].items():
        L.append(f"    {k:<18} {e['status']:<14} process={e['process'] or '-'}  settings={e['settings'] or '-'}")
    c = out["cks_config"]
    L.append(f"  cks_config : {c['path'] or '-'}  exists={c['exists']}  (source_root/freshness: live probe)")
    pm = out["permissions"]
    L.append(f"  permissions: defaultMode={pm['defaultMode']} plugin_allowlisted={pm['plugin_allowlisted']} (allow={pm['allow_count']})")
    if out["restart_needed"]:
        L.append(f"  ⚠ restart needed (in settings, not in current env): {', '.join(out['restart_needed'])}")
    if out["issues"]:
        L.append("  issues:")
        L += [f"    - {i}" for i in out["issues"]]
    return "\n".join(L)


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description="coding-agent environment diagnostics (read-only)")
    ap.add_argument("--plugin-root", default=os.environ.get("CLAUDE_PLUGIN_ROOT", ""))
    ap.add_argument("--project-id", "--project", dest="project_id", default=None)
    ap.add_argument("--json", action="store_true")
    a = ap.parse_args(argv)
    out = diagnose(Path(a.plugin_root) if a.plugin_root else None, a.project_id)
    print(json.dumps(out, indent=2) if a.json else render(out))
    return 0 if out["verdict"] == "READY" else 1


if __name__ == "__main__":
    raise SystemExit(main())
