#!/usr/bin/env python3
"""Tests for doctor.py — the read-only environment diagnostics helper.

Run:  python3 plugin/scripts/tests/test_doctor.py
"""
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

_SCRIPTS = Path(__file__).resolve().parent.parent
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

import doctor  # noqa: E402

DOCTOR_PY = _SCRIPTS / "doctor.py"


def _make_plugin_root(tmp: Path, packs: dict) -> Path:
    """packs = {project_id: repo_root_env}. Build a minimal plugin-root."""
    root = tmp / "plugin-root"
    (root / ".claude-plugin").mkdir(parents=True)
    (root / ".claude-plugin" / "plugin.json").write_text(json.dumps({"version": "9.9.9"}))
    for pid, rre in packs.items():
        d = root / "domains" / pid
        d.mkdir(parents=True)
        (d / "domain-pack.json").write_text(json.dumps(
            {"project_id": pid, "verification": {"repo_root_env": rre}}))
    return root


class TestDetectProjectId(unittest.TestCase):
    def test_single_pack(self):
        with tempfile.TemporaryDirectory() as d:
            root = _make_plugin_root(Path(d), {"go-stablenet": "GO_STABLENET_ROOT"})
            pid, packs = doctor.detect_project_id(root, "", None)
            self.assertEqual(pid, "go-stablenet")
            self.assertEqual(packs, ["go-stablenet"])

    def test_override_wins(self):
        with tempfile.TemporaryDirectory() as d:
            root = _make_plugin_root(Path(d), {"a": "A_ROOT", "b": "B_ROOT"})
            pid, _ = doctor.detect_project_id(root, "", "b")
            self.assertEqual(pid, "b")

    def test_multi_name_match(self):
        with tempfile.TemporaryDirectory() as d:
            root = _make_plugin_root(Path(d), {"alpha": "A", "beta": "B"})
            pid, _ = doctor.detect_project_id(root, "/work/repos/beta-service", None)
            self.assertEqual(pid, "beta")

    def test_multi_no_match_none(self):
        with tempfile.TemporaryDirectory() as d:
            root = _make_plugin_root(Path(d), {"alpha": "A", "beta": "B"})
            pid, _ = doctor.detect_project_id(root, "/work/repos/zeta", None)
            self.assertIsNone(pid)


class TestSmoke(unittest.TestCase):
    def test_json_report_structure(self):
        with tempfile.TemporaryDirectory() as d:
            root = _make_plugin_root(Path(d), {"go-stablenet": "GO_STABLENET_ROOT"})
            r = subprocess.run(
                [sys.executable, str(DOCTOR_PY), "--plugin-root", str(root), "--json"],
                cwd=d, capture_output=True, text=True)
            out = json.loads(r.stdout)
            for key in ("plugin", "project", "domain_pack", "env", "cks_config",
                        "permissions", "verdict", "issues", "restart_needed"):
                self.assertIn(key, out)
            self.assertEqual(out["plugin"]["active_version"], "9.9.9")
            self.assertEqual(out["domain_pack"]["project_id"], "go-stablenet")
            self.assertEqual(out["domain_pack"]["repo_root_env"], "GO_STABLENET_ROOT")
            # GO_STABLENET_ROOT unset in this sandbox -> reported in env table
            self.assertIn("GO_STABLENET_ROOT", out["env"])

    def test_secret_masked(self):
        self.assertEqual(doctor._mask("JIRA_API_TOKEN", "supersecret"), "********")
        self.assertEqual(doctor._mask("CKS_CONFIG", "/path"), "/path")


if __name__ == "__main__":
    unittest.main(verbosity=2)
