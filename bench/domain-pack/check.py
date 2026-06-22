#!/usr/bin/env python3
"""check.py — validate the domain-pack structure (overlay P1 Phase 1 gate).

Phase 1 moved go-stablenet domain content into plugin/domains/go-stablenet/ and made
the stablenet-* skills thin pointers, introducing the generic domain-pack loader.
This gate locks that structure so a later edit can't silently break it:

  - each plugin/domains/<id>/domain-pack.json has the required keys, and its
    referenced files (invariants, context_classifier) exist;
  - the generic `domain-pack` loader skill exists;
  - the stablenet-* pointer skills actually point at the domains/ files
    (so there is one source, not a stale duplicate).

It does NOT yet check the Phase-2 acceptance (core grep-clean / no-regression);
those land when agents are rewired. Pure structure check, no LLM.

    python3 bench/domain-pack/check.py
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]                       # bench/domain-pack -> repo root
DOMAINS = REPO / "plugin" / "domains"
SKILLS = REPO / "plugin" / "skills"

REQUIRED_KEYS = ("project_id", "ticket_namespace", "invariants", "context_classifier", "knowledge")
# pointer skill -> the domains file it must reference (Phase 1 single-source guard)
POINTERS = {
    "stablenet-invariants": "domains/go-stablenet/invariants.md",
    "stablenet-context": "domains/go-stablenet/context.md",
}


def check(*, domains_dir: Path = DOMAINS, skills_dir: Path = SKILLS,
          check_pointers: bool = True) -> int:
    problems: list[str] = []
    packs = sorted(domains_dir.glob("*/domain-pack.json"))
    if not packs:
        problems.append(f"no domain packs under {domains_dir}")

    for pack_path in packs:
        pid = pack_path.parent.name
        try:
            doc = json.loads(pack_path.read_text())
        except json.JSONDecodeError as e:
            problems.append(f"{pack_path}: invalid JSON ({e})")
            continue
        for key in REQUIRED_KEYS:
            if key not in doc:
                problems.append(f"{pid}: domain-pack.json missing required key '{key}'")
        if doc.get("project_id") not in (pid, None) and "project_id" in doc:
            if doc["project_id"] != pid:
                problems.append(f"{pid}: project_id '{doc['project_id']}' != directory name")
        # referenced files exist
        for ref_key in ("invariants", "context_classifier"):
            ref = doc.get(ref_key)
            if ref and not (pack_path.parent / ref).is_file():
                problems.append(f"{pid}: {ref_key} -> {ref} not found in {pack_path.parent}")

    # generic loader skill present
    if not (skills_dir / "domain-pack" / "SKILL.md").is_file():
        problems.append(f"generic loader skill missing: {skills_dir}/domain-pack/SKILL.md")

    # pointer skills point at the domains files (single source)
    if check_pointers:
        for skill, must_ref in POINTERS.items():
            sp = skills_dir / skill / "SKILL.md"
            if not sp.is_file():
                problems.append(f"pointer skill missing: {sp}")
            elif must_ref not in sp.read_text():
                problems.append(f"{skill} skill does not reference '{must_ref}' (stale duplicate?)")

    if problems:
        print(f"DOMAIN-PACK STRUCTURE PROBLEMS ({len(problems)}):")
        for p in problems:
            print(f"  - {p}")
        return 1
    names = ", ".join(p.parent.name for p in packs)
    print(f"domain-pack structure OK — packs: [{names}]; loader + pointers conform")
    return 0


def main(argv: list[str] | None = None) -> int:
    argparse.ArgumentParser(description="validate domain-pack structure").parse_args(argv)
    return check()


if __name__ == "__main__":
    raise SystemExit(main())
