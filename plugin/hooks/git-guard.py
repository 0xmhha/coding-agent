#!/usr/bin/env python3
"""Git-safety PreToolUse guard (coding-agent plugin).

Turns the prose git-safety rules (no direct commit/push to the default branch,
no force-push, push/merge only when asked) into a deterministic hook the model
cannot forget — the fail-closed gate that makes autonomy=auto safe to enable.
Generic across repos.

Decisions (PreToolUse on Bash):
  deny  — force-push; push to a protected branch (main/master); commit while the
          working tree is on a protected branch.
  ask   — destructive history/tree ops (reset --hard, clean -f[d], branch -D) and
          tag / release pushes (relaxed to allow only when the active workspace
          has autonomy.auto_merge == true).
  (allow) — anything else, OR any parse failure: the guard fires ONLY on a
            positively-matched dangerous pattern, never on uncertainty about an
            unrelated command, so it can't break normal work.

Communicates via JSON on stdout (permissionDecision). Branch-dependent checks run
git in the hook's cwd; an explicit `git -C <other-repo>` skips the cwd branch
check (the target tree is unknown to the hook).
"""
import sys
import json
import re
import os
import glob
import subprocess

PROTECTED = ("main", "master")


def emit(decision, reason):
    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": decision,
            "permissionDecisionReason": reason,
        }
    }))


def _git(args):
    try:
        out = subprocess.run(["git"] + args, capture_output=True, text=True, timeout=2)
        return out.stdout.strip()
    except Exception:
        return ""


def _auto_merge_enabled():
    """True if any recent active workspace opted into auto_merge (relaxes tag push)."""
    root = _git(["rev-parse", "--show-toplevel"]) or os.getcwd()
    for p in glob.glob(os.path.join(root, ".coding-agent", "tickets", "*", "state.json")):
        try:
            st = json.load(open(p, encoding="utf-8"))
        except Exception:
            continue
        if ((st.get("config") or {}).get("autonomy") or {}).get("auto_merge") is True:
            return True
    return False


def main():
    try:
        data = json.load(sys.stdin)
    except Exception:
        return 0
    if data.get("tool_name") != "Bash":
        return 0
    cmd = ((data.get("tool_input") or {}).get("command") or "")
    # Only inspect commands that actually invoke git (at start or after a separator).
    if not cmd or not re.search(r'(^|[;&|])\s*git(\s|$)', cmd):
        return 0

    is_push = re.search(r'\bgit\b[^;&|]*\bpush\b', cmd) is not None

    # --- deny: force-push (rewrites shared history) ---
    if is_push and re.search(r'(--force(?!-with-lease)\b|(^|\s)-f(\s|$))', cmd):
        emit("deny",
             "Force-push is blocked — it rewrites shared history. If a force-push is "
             "genuinely required, run it yourself outside the agent.")
        return 0

    # --- deny: push to a protected branch ---
    if is_push and re.search(r'\bpush\b[^;&|]*(?:\s|:)(?:main|master)(?:\s|:|$)', cmd):
        emit("deny",
             "Direct push to a protected branch (main/master) is blocked. Push a feature "
             "branch and open a PR; merge to the default branch happens via review.")
        return 0

    # --- deny: commit while ON a protected branch (cwd tree only; -C skips) ---
    if re.search(r'\bgit\b[^;&|]*\bcommit\b', cmd) and not re.search(r'\bgit\s+-C\b', cmd):
        branch = _git(["rev-parse", "--abbrev-ref", "HEAD"])
        if branch in PROTECTED:
            emit("deny",
                 f"Committing directly on '{branch}' is blocked. Create a feature branch "
                 f"first (git checkout -b <branch>) and commit there.")
            return 0

    # --- ask: destructive history/tree ops ---
    if re.search(r'\breset\b[^;&|]*--hard', cmd):
        emit("ask", "`git reset --hard` discards uncommitted work and rewrites the branch "
                    "tip. Confirm this is intended.")
        return 0
    if re.search(r'\bclean\b\s+-[a-z]*f', cmd):
        emit("ask", "`git clean -f` permanently deletes untracked files. Confirm the target "
                    "and that nothing valuable is untracked.")
        return 0
    if re.search(r'\bbranch\b[^;&|]*\s-D\b', cmd):
        emit("ask", "`git branch -D` force-deletes a branch (even unmerged). Confirm it is "
                    "merged or disposable.")
        return 0

    # --- ask: tag / release push (relaxed when auto_merge is enabled) ---
    if is_push and re.search(r'(--tags\b|\btag\b)', cmd):
        if not _auto_merge_enabled():
            emit("ask", "Pushing tags publishes a release ref. Confirm the tag and target "
                        "(or enable autonomy.auto_merge for release automation).")
            return 0

    return 0


if __name__ == "__main__":
    sys.exit(main())
