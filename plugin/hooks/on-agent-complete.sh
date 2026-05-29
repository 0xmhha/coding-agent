#!/usr/bin/env bash
# coding-agent — PostToolUse hook for the Agent tool.
#
# Fires after a sub-agent (orchestrator/planner/implementer/evaluator) finishes.
# Reads the hook payload from stdin (Claude Code passes JSON), extracts the
# tool result, and appends a structured log line to every active workspace
# under .coding-agent/tickets/.
#
# The hook never fails the pipeline. It exits 0 even when logging is impossible
# (e.g., outside a git repo) so the agent run is never blocked by hook errors.

set -u

# Read the JSON payload (best-effort; if jq isn't installed we use grep).
payload="$(cat 2>/dev/null || true)"

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [ -z "${repo_root}" ]; then
  exit 0
fi

tickets_dir="${repo_root}/.coding-agent/tickets"
[ -d "${tickets_dir}" ] || exit 0

ts="$(date -u +%FT%TZ)"

# Try to extract the agent's subagent_type from the payload.
subagent_type="unknown"
if command -v jq >/dev/null 2>&1; then
  extracted="$(printf '%s' "${payload}" | jq -r '.tool_input.subagent_type // empty' 2>/dev/null || true)"
  [ -n "${extracted}" ] && subagent_type="${extracted}"
fi

# Append to the impl.log of every active workspace.
# A workspace is "active" if it has a state.json and current_state != COMPLETED.
for ws in "${tickets_dir}"/*/ ; do
  [ -d "${ws}" ] || continue
  state_file="${ws}state.json"
  [ -f "${state_file}" ] || continue

  current_state=""
  if command -v jq >/dev/null 2>&1; then
    current_state="$(jq -r '.current_state // empty' "${state_file}" 2>/dev/null || true)"
  fi
  if [ "${current_state}" = "COMPLETED" ]; then
    continue
  fi

  mkdir -p "${ws}logs"
  printf '%s [INFO] hook=on-agent-complete subagent=%s\n' \
    "${ts}" "${subagent_type}" >> "${ws}logs/impl.log" 2>/dev/null || true
done

exit 0
