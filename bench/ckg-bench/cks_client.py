"""cks_client.py — MCP stdio client for the Code Knowledge System (cks).

Wraps the ``cks-mcp`` binary as a long-lived subprocess and exposes a
single callable that matches the ``cks_tool(tool_name, args_dict) -> dict``
contract that methods/scorers expect.

Short name → full MCP tool name mapping
-----------------------------------------
  get_for_task     → cks.context.get_for_task
  get_subgraph     → cks.context.get_subgraph
  find_symbol      → cks.context.find_symbol
  find_callers     → cks.context.find_callers
  semantic_search  → cks.context.semantic_search
  search_text      → cks.context.search_text
  find_callees     → cks.context.find_callees

Usage::

    with CKSClient(bin_path, config_path) as cks:
        result = cks("get_for_task", {"prompt": "..."})
        symbol = cks("find_symbol", {"name": "QuorumSize"})

On any failure the callable returns a structured error dict (never raises),
so methods degrade gracefully to ``cks_partial``.

The client is a context manager that spawns the server once and reuses the
process across calls.  ``__exit__`` sends SIGTERM and waits up to 3 s.
"""

from __future__ import annotations

import json
import os
import subprocess
import threading
from typing import Any, Dict, Optional


# Map short names used by methods to fully-qualified MCP tool names
_TOOL_NAME_MAP: Dict[str, str] = {
    "get_for_task":    "cks.context.get_for_task",
    "get_subgraph":    "cks.context.get_subgraph",
    "find_symbol":     "cks.context.find_symbol",
    "find_callers":    "cks.context.find_callers",
    "semantic_search": "cks.context.semantic_search",
    "search_text":     "cks.context.search_text",
    "find_callees":    "cks.context.find_callees",
    "change_history":  "cks.context.change_history",
    "impact_analysis": "cks.context.impact_analysis",
    "concurrency_impact": "cks.context.concurrency_impact",
    # Allow fully-qualified names to pass through unchanged
    "cks.context.get_for_task":       "cks.context.get_for_task",
    "cks.context.get_subgraph":       "cks.context.get_subgraph",
    "cks.context.find_symbol":        "cks.context.find_symbol",
    "cks.context.find_callers":       "cks.context.find_callers",
    "cks.context.semantic_search":    "cks.context.semantic_search",
    "cks.context.search_text":        "cks.context.search_text",
    "cks.context.find_callees":       "cks.context.find_callees",
    "cks.context.change_history":     "cks.context.change_history",
    "cks.context.impact_analysis":    "cks.context.impact_analysis",
    "cks.context.concurrency_impact": "cks.context.concurrency_impact",
}

_DEFAULT_TIMEOUT = 30  # seconds per tool call


class CKSClient:
    """MCP stdio client for cks-mcp.

    Parameters
    ----------
    bin_path : str
        Path to the ``cks-mcp`` binary.
    config_path : str
        Path to the cks YAML config file.
    timeout : int
        Per-call timeout in seconds. Default 30.
    """

    def __init__(
        self,
        bin_path: str,
        config_path: str,
        timeout: int = _DEFAULT_TIMEOUT,
    ) -> None:
        self._bin_path = bin_path
        self._config_path = config_path
        self._timeout = timeout
        self._proc: Optional[subprocess.Popen] = None
        self._lock = threading.Lock()
        self._req_id = 0

    # ------------------------------------------------------------------
    # Context manager
    # ------------------------------------------------------------------

    def __enter__(self) -> "CKSClient":
        self._start()
        return self

    def __exit__(self, *_: Any) -> None:
        self._stop()

    # ------------------------------------------------------------------
    # Internal lifecycle
    # ------------------------------------------------------------------

    def _start(self) -> None:
        """Spawn the cks-mcp subprocess and run the MCP handshake."""
        self._proc = subprocess.Popen(
            [self._bin_path, "-config", self._config_path],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        # MCP initialize handshake
        self._send_raw({
            "jsonrpc": "2.0",
            "id": self._next_id(),
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "ckg-bench", "version": "0.1"},
            },
        })
        # Read initialize response (discard, just drain it)
        self._read_line()
        # Send notifications/initialized (no response expected)
        self._send_raw({
            "jsonrpc": "2.0",
            "method": "notifications/initialized",
            "params": {},
        })

    def _stop(self) -> None:
        """Terminate the subprocess cleanly."""
        if self._proc is None:
            return
        try:
            self._proc.stdin.close()
        except OSError:
            pass
        try:
            self._proc.wait(timeout=3)
        except subprocess.TimeoutExpired:
            self._proc.kill()
        self._proc = None

    def _next_id(self) -> int:
        self._req_id += 1
        return self._req_id

    def _send_raw(self, msg: Dict[str, Any]) -> None:
        assert self._proc is not None
        line = json.dumps(msg, ensure_ascii=False) + "\n"
        self._proc.stdin.write(line.encode("utf-8"))
        self._proc.stdin.flush()

    def _read_line(self) -> str:
        """Read one line from stdout with timeout via readline (blocking)."""
        assert self._proc is not None
        # readline() blocks until newline or EOF; timeout is handled at the
        # call-site via threading if needed.
        line = self._proc.stdout.readline()
        return line.decode("utf-8").strip() if line else ""

    # ------------------------------------------------------------------
    # Tool call
    # ------------------------------------------------------------------

    def _call(self, full_tool_name: str, arguments: Dict[str, Any]) -> Dict[str, Any]:
        """Send a tools/call request and return the parsed response."""
        req_id = self._next_id()
        self._send_raw({
            "jsonrpc": "2.0",
            "id": req_id,
            "method": "tools/call",
            "params": {"name": full_tool_name, "arguments": arguments},
        })

        raw = self._read_line()
        if not raw:
            return {"error": "cks returned empty response"}

        try:
            resp = json.loads(raw)
        except json.JSONDecodeError as exc:
            return {"error": f"cks JSON decode error: {exc}", "raw": raw[:200]}

        if "error" in resp:
            err = resp["error"]
            msg = err.get("message", str(err)) if isinstance(err, dict) else str(err)
            return {"error": f"cks RPC error: {msg}"}

        result = resp.get("result", {})
        # Prefer structuredContent (machine-readable) over content[].text
        structured = result.get("structuredContent")
        if structured is not None:
            return structured

        # Fall back to content[0].text if no structuredContent
        content = result.get("content", [])
        if content:
            text = content[0].get("text", "")
            # Try to parse as JSON
            try:
                return json.loads(text)
            except (json.JSONDecodeError, TypeError):
                return {"text": text}

        return {"error": "cks returned no content"}

    # ------------------------------------------------------------------
    # Public callable interface
    # ------------------------------------------------------------------

    def __call__(self, tool_name: str, args: Dict[str, Any]) -> Dict[str, Any]:
        """Call a cks tool by short or full name.

        Parameters
        ----------
        tool_name : str
            Short name (e.g. ``"get_for_task"``) or fully qualified name
            (e.g. ``"cks.context.get_for_task"``).
        args : dict
            Arguments to pass to the tool.

        Returns
        -------
        dict
            Parsed tool result, or ``{"error": "<reason>"}`` on failure.
            Never raises.
        """
        full_name = _TOOL_NAME_MAP.get(tool_name)
        if full_name is None:
            return {"error": f"unknown cks tool: {tool_name!r}"}

        if self._proc is None or self._proc.poll() is not None:
            return {"error": "cks subprocess is not running"}

        with self._lock:
            try:
                # Use a thread with timeout to avoid blocking forever
                result: Dict[str, Any] = {}
                exc_holder: list = []

                def _do() -> None:
                    try:
                        result.update(self._call(full_name, args))
                    except Exception as e:
                        exc_holder.append(e)

                t = threading.Thread(target=_do, daemon=True)
                t.start()
                t.join(timeout=self._timeout)
                if t.is_alive():
                    return {"error": f"cks tool call timed out after {self._timeout}s"}
                if exc_holder:
                    return {"error": f"cks tool call error: {exc_holder[0]}"}
                return result
            except Exception as exc:
                return {"error": f"cks client unexpected error: {exc}"}


def make_cks_client_from_env(timeout: int = _DEFAULT_TIMEOUT) -> Optional[CKSClient]:
    """Construct a CKSClient from environment variables.

    Reads ``CKS_MCP_BIN`` and ``CKS_CONFIG``.  Returns None if either is
    unset, so callers can degrade gracefully.
    """
    bin_path = os.environ.get("CKS_MCP_BIN", "").strip()
    config_path = os.environ.get("CKS_CONFIG", "").strip()
    if not bin_path or not config_path:
        return None
    return CKSClient(bin_path=bin_path, config_path=config_path, timeout=timeout)
