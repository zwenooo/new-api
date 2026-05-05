#!/usr/bin/env python3
"""
Lint the mixed-binary handoff docs for artifact-set consistency.

This is a local doc-consistency check for the Phase 5 external-lane scaffold.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent.parent
MANIFEST_PATH = SCRIPT_DIR / "mixed_binary_scaffold_manifest.json"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Check mixed-binary handoff docs for artifact-list consistency."
    )
    parser.add_argument(
        "--root",
        default=str(REPO_ROOT),
        help=f"Repository root (default: {REPO_ROOT})",
    )
    return parser.parse_args()


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def load_expected_artifacts() -> list[str]:
    data = json.loads(MANIFEST_PATH.read_text(encoding="utf-8"))
    artifacts = data.get("expected_doc_artifacts", [])
    if not isinstance(artifacts, list) or not all(isinstance(item, str) for item in artifacts):
        raise SystemExit(f"invalid expected_doc_artifacts list in manifest: {MANIFEST_PATH}")
    return artifacts


def main() -> int:
    args = parse_args()
    root = Path(args.root)
    expected_artifacts = load_expected_artifacts()

    docs = {
        "evidence_readme": root / ".omx/plans/evidence/README.md",
        "mixed_binary_readme": root / ".omx/plans/evidence/reclaim-phase0/mixed-binary/README.md",
        "phase5_status": root / ".omx/plans/evidence/reclaim-phase5-status.md",
        "phase5_handoff": root / ".omx/plans/evidence/reclaim-phase5-rollout-handoff.md",
        "status_index": root / ".omx/plans/evidence/reclaim-status-index.md",
    }

    errors: list[str] = []
    mentions: dict[str, set[str]] = {}

    for name, path in docs.items():
        if not path.exists():
            errors.append(f"missing doc: {path}")
            continue
        text = read_text(path)
        mentions[name] = {artifact for artifact in expected_artifacts if artifact in text}

    if errors:
        print(json.dumps({"ok": False, "errors": errors}, ensure_ascii=True, indent=2))
        return 1

    for artifact in expected_artifacts:
        missing_from = [name for name, seen in mentions.items() if artifact not in seen]
        if missing_from:
            errors.append(f"{artifact} missing from: {', '.join(missing_from)}")

    print(
        json.dumps(
            {
                "ok": not errors,
                "checked_docs": {name: str(path) for name, path in docs.items()},
                "errors": errors,
            },
            ensure_ascii=True,
            indent=2,
        )
    )
    return 0 if not errors else 1


if __name__ == "__main__":
    raise SystemExit(main())
