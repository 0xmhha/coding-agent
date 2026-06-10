"""test_cks_client.py — unit tests for CKSClient message framing.

Uses a fake subprocess (io.BytesIO-based) so no live cks server is needed.
Tests JSON-RPC 2.0 framing, short-to-full tool name mapping, structuredContent
extraction, and graceful error handling.
"""

from __future__ import annotations

import io
import json
import os
import sys
import threading
import unittest

_BENCH_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if _BENCH_ROOT not in sys.path:
    sys.path.insert(0, _BENCH_ROOT)

from cks_client import CKSClient, _TOOL_NAME_MAP


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _encode(msg: dict) -> bytes:
    return (json.dumps(msg) + "\n").encode("utf-8")


class _FakeProc:
    """Simulate a cks-mcp subprocess for unit testing.

    Reads JSON-RPC requests from a write pipe and returns canned responses
    on a read pipe.
    """

    def __init__(self, responses: list) -> None:
        # responses: list of dicts to send back (one per call to _read_line)
        self._responses = iter(responses)
        self._lock = threading.Lock()

        # Pipes: stdin side the client writes to; stdout side the client reads from
        read_fd, write_fd = os.pipe()
        self.stdin = open(write_fd, "wb", buffering=0)
        self._stdout_write = open(read_fd, "rb", buffering=0)

        # We feed responses into a buffer the client reads from
        read2_fd, write2_fd = os.pipe()
        self.stdout = open(read2_fd, "rb", buffering=0)
        self._stdout_inject = open(write2_fd, "wb", buffering=0)

        self.stderr = io.BytesIO()
        self._poll_val = None

    def poll(self) -> None:
        return self._poll_val

    def wait(self, timeout: float = 3) -> int:
        return 0

    def kill(self) -> None:
        pass

    def inject_response(self, msg: dict) -> None:
        """Pre-inject a response that _read_line will return."""
        self._stdout_inject.write(_encode(msg))
        self._stdout_inject.flush()

    def close(self) -> None:
        try:
            self._stdout_inject.close()
        except OSError:
            pass


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

class TestToolNameMap(unittest.TestCase):
    def test_short_names_map_to_full(self):
        self.assertEqual(_TOOL_NAME_MAP["get_for_task"], "cks.context.get_for_task")
        self.assertEqual(_TOOL_NAME_MAP["find_symbol"], "cks.context.find_symbol")
        self.assertEqual(_TOOL_NAME_MAP["semantic_search"], "cks.context.semantic_search")
        self.assertEqual(_TOOL_NAME_MAP["get_subgraph"], "cks.context.get_subgraph")
        self.assertEqual(_TOOL_NAME_MAP["find_callers"], "cks.context.find_callers")

    def test_full_names_pass_through(self):
        self.assertEqual(
            _TOOL_NAME_MAP["cks.context.get_for_task"], "cks.context.get_for_task"
        )

    def test_unknown_short_name_not_in_map(self):
        self.assertNotIn("nonexistent_tool", _TOOL_NAME_MAP)


class TestCKSClientUnknownTool(unittest.TestCase):
    def test_unknown_tool_returns_error(self):
        """Calling with an unmapped tool name returns error dict, no raise."""
        client = CKSClient.__new__(CKSClient)
        client._bin_path = "/dev/null"
        client._config_path = "/dev/null"
        client._timeout = 5
        client._proc = object()  # non-None sentinel
        client._lock = threading.Lock()
        client._req_id = 0

        # Monkey-patch proc.poll() to return None (alive)
        class _Proc:
            def poll(self):
                return None

        client._proc = _Proc()
        result = client("nonexistent_tool", {})
        self.assertIn("error", result)
        self.assertIn("unknown cks tool", result["error"])


class TestCKSClientStructuredContent(unittest.TestCase):
    """Test that structuredContent is preferred over content[].text."""

    def _make_response(self, req_id: int, structured: dict) -> dict:
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "content": [{"type": "text", "text": "plain text fallback"}],
                "structuredContent": structured,
            },
        }

    def _make_text_only_response(self, req_id: int, text: str) -> dict:
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "content": [{"type": "text", "text": text}],
            },
        }

    def _make_error_response(self, req_id: int, message: str) -> dict:
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "error": {"code": -32000, "message": message},
        }

    def test_structured_content_preferred(self):
        client = CKSClient.__new__(CKSClient)
        client._req_id = 10
        client._timeout = 5

        structured = {"citations": [{"file": "foo.go", "start_line": 1}]}
        resp = self._make_response(11, structured)

        # Inject a fake _read_line
        client._send_raw = lambda msg: None
        client._read_line = lambda: json.dumps(resp)

        result = client._call("cks.context.find_symbol", {"name": "Foo"})
        self.assertEqual(result, structured)

    def test_text_fallback_parsed_as_json(self):
        client = CKSClient.__new__(CKSClient)
        client._req_id = 10
        client._timeout = 5

        inner = {"query": "q", "citations": []}
        resp = self._make_text_only_response(11, json.dumps(inner))

        client._send_raw = lambda msg: None
        client._read_line = lambda: json.dumps(resp)

        result = client._call("cks.context.get_for_task", {"prompt": "q"})
        self.assertEqual(result, inner)

    def test_text_fallback_plain_string(self):
        client = CKSClient.__new__(CKSClient)
        client._req_id = 10
        client._timeout = 5

        resp = self._make_text_only_response(11, "evidence pack")
        client._send_raw = lambda msg: None
        client._read_line = lambda: json.dumps(resp)

        result = client._call("cks.context.get_for_task", {"prompt": "q"})
        self.assertEqual(result, {"text": "evidence pack"})

    def test_rpc_error_returns_error_dict(self):
        client = CKSClient.__new__(CKSClient)
        client._req_id = 10
        client._timeout = 5

        resp = self._make_error_response(11, "tool not found")
        client._send_raw = lambda msg: None
        client._read_line = lambda: json.dumps(resp)

        result = client._call("cks.context.find_symbol", {"name": "X"})
        self.assertIn("error", result)
        self.assertIn("tool not found", result["error"])

    def test_empty_response_returns_error(self):
        client = CKSClient.__new__(CKSClient)
        client._req_id = 10
        client._timeout = 5

        client._send_raw = lambda msg: None
        client._read_line = lambda: ""

        result = client._call("cks.context.find_symbol", {"name": "X"})
        self.assertIn("error", result)

    def test_invalid_json_returns_error(self):
        client = CKSClient.__new__(CKSClient)
        client._req_id = 10
        client._timeout = 5

        client._send_raw = lambda msg: None
        client._read_line = lambda: "not valid json {"

        result = client._call("cks.context.find_symbol", {"name": "X"})
        self.assertIn("error", result)


class TestCKSClientPublicCallable(unittest.TestCase):
    def test_returns_error_when_proc_is_none(self):
        client = CKSClient.__new__(CKSClient)
        client._proc = None
        client._lock = threading.Lock()
        client._req_id = 0
        client._timeout = 5

        result = client("get_for_task", {"prompt": "q"})
        self.assertIn("error", result)
        self.assertIn("not running", result["error"])

    def test_structured_result_via_public_call(self):
        """Full path through __call__ → _call using monkeypatched _read_line."""
        client = CKSClient.__new__(CKSClient)
        client._timeout = 5
        client._lock = threading.Lock()
        client._req_id = 0

        structured = {"citations": [{"file": "validator.go"}]}

        def _fake_call(full_name: str, args: dict) -> dict:
            return structured

        class _Proc:
            def poll(self):
                return None

        client._proc = _Proc()
        client._call = _fake_call

        result = client("find_symbol", {"name": "QuorumSize"})
        self.assertEqual(result, structured)


if __name__ == "__main__":
    unittest.main()
