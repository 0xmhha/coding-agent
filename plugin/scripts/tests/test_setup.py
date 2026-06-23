#!/usr/bin/env python3
"""Tests for setup.py extensions — repo_root_env resolution + allowlist merge.

Run:  python3 plugin/scripts/tests/test_setup.py
"""
import json
import sys
import tempfile
import unittest
from pathlib import Path

_SCRIPTS = Path(__file__).resolve().parent.parent
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

import setup  # noqa: E402


def _plugin_root(tmp: Path, packs: dict) -> Path:
    root = tmp / "plugin-root"
    for pid, rre in packs.items():
        d = root / "domains" / pid
        d.mkdir(parents=True)
        (d / "domain-pack.json").write_text(json.dumps(
            {"project_id": pid, "verification": {"repo_root_env": rre}}))
    return root


class TestRepoRootEnv(unittest.TestCase):
    def test_single_pack(self):
        with tempfile.TemporaryDirectory() as d:
            root = _plugin_root(Path(d), {"go-stablenet": "GO_STABLENET_ROOT"})
            self.assertEqual(setup._repo_root_env(root, Path("/x/repo"), None), "GO_STABLENET_ROOT")

    def test_override(self):
        with tempfile.TemporaryDirectory() as d:
            root = _plugin_root(Path(d), {"a": "A_ROOT", "b": "B_ROOT"})
            self.assertEqual(setup._repo_root_env(root, Path("/x/repo"), "b"), "B_ROOT")

    def test_name_match(self):
        with tempfile.TemporaryDirectory() as d:
            root = _plugin_root(Path(d), {"alpha": "AL", "beta": "BE"})
            self.assertEqual(setup._repo_root_env(root, Path("/x/beta-svc"), None), "BE")

    def test_no_pack_none(self):
        with tempfile.TemporaryDirectory() as d:
            root = _plugin_root(Path(d), {"alpha": "AL", "beta": "BE"})
            self.assertIsNone(setup._repo_root_env(root, Path("/x/zeta"), None))


class TestMergeAllow(unittest.TestCase):
    def test_merge_and_dedup(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "settings.local.json"
            added1 = setup._merge_allow(p, ["mcp__x__*", "Bash(ls:*)"])
            self.assertEqual(set(added1), {"mcp__x__*", "Bash(ls:*)"})
            # re-merge: nothing new (dedup)
            added2 = setup._merge_allow(p, ["mcp__x__*", "Bash(cat:*)"])
            self.assertEqual(added2, ["Bash(cat:*)"])
            doc = json.loads(p.read_text())
            self.assertEqual(doc["permissions"]["allow"], ["mcp__x__*", "Bash(ls:*)", "Bash(cat:*)"])

    def test_preserves_existing_doc(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "settings.local.json"
            p.write_text(json.dumps({"env": {"FOO": "bar"}, "permissions": {"allow": ["keep"]}}))
            setup._merge_allow(p, ["new"])
            doc = json.loads(p.read_text())
            self.assertEqual(doc["env"], {"FOO": "bar"})          # untouched
            self.assertEqual(doc["permissions"]["allow"], ["keep", "new"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
