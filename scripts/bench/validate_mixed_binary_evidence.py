#!/usr/bin/env python3
"""
Validate the mixed-binary evidence scaffold for the external MySQL + Redis rollout lane.

Default mode checks scaffold files and JSON parseability and reports warnings for
placeholder / empty fields. Use:
- --strict-placeholders to fail on placeholder warnings
- --require-runtime to also require runtime capture artifacts and at least one
  Phase 3 runner JSON artifact
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent.parent
MANIFEST_PATH = SCRIPT_DIR / "mixed_binary_scaffold_manifest.json"
DEFAULT_DIR = REPO_ROOT / ".omx/plans/evidence/reclaim-phase0/mixed-binary"
DEFAULT_PHASE3_DIR = REPO_ROOT / ".omx/plans/evidence/reclaim-phase3"

RUNTIME_FILES = [
    "compose-ps.txt",
    "compose-logs.txt",
    "docker-stats.txt",
    "cache-revision-check.txt",
    "option-sync-check.txt",
]

PLACEHOLDER_TOKENS = (
    "<old-image-or-commit>",
    "<new-image-or-commit>",
    "<compose-file>",
    "<env-file>",
    "<prod-like-mysql-or-mixed-binary-note>",
    "<fill-me>",
    "placeholder",
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate mixed-binary evidence completeness and placeholder leakage."
    )
    parser.add_argument(
        "--dir",
        default=str(DEFAULT_DIR),
        help=f"Mixed-binary evidence directory (default: {DEFAULT_DIR})",
    )
    parser.add_argument(
        "--strict-placeholders",
        action="store_true",
        help="Fail when placeholder or empty values are still present in identity/topology fields.",
    )
    parser.add_argument(
        "--require-runtime",
        action="store_true",
        help="Also require runtime capture artifacts and at least one Phase 3 runner JSON output.",
    )
    parser.add_argument(
        "--phase3-dir",
        default=str(DEFAULT_PHASE3_DIR),
        help=f"Phase 3 evidence directory for runner JSON checks (default: {DEFAULT_PHASE3_DIR})",
    )
    return parser.parse_args()


def contains_placeholder(value: str) -> bool:
    lowered = value.strip().lower()
    if not lowered:
        return True
    return any(token in lowered for token in PLACEHOLDER_TOKENS)


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8").strip()


def read_json(path: Path) -> dict:
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def load_required_files() -> list[str]:
    data = read_json(MANIFEST_PATH)
    required = data.get("required_files", [])
    if not isinstance(required, list) or not all(isinstance(item, str) for item in required):
        raise SystemExit(f"invalid required_files list in manifest: {MANIFEST_PATH}")
    return required


def validate_generated_summary(path: Path, errors: list[str]) -> None:
    if not path.exists():
        return
    content = read_text(path)
    if not content:
        errors.append(f"generated result summary is empty: {path}")
        return
    if "Pending generation." in content:
        errors.append(f"generated result summary is still a placeholder stub: {path}")
        return
    if "## Validator Snapshot" not in content:
        errors.append(f"generated result summary is missing the validator snapshot section: {path}")


def main() -> int:
    args = parse_args()
    root = Path(args.dir)
    phase3_dir = Path(args.phase3_dir)
    required_files = load_required_files()

    errors: list[str] = []
    warnings: list[str] = []

    if not root.exists():
        errors.append(f"missing evidence directory: {root}")
        print(json.dumps({"ok": False, "errors": errors, "warnings": warnings}, ensure_ascii=True, indent=2))
        return 1

    for name in required_files:
        path = root / name
        if not path.exists():
            errors.append(f"missing required file: {path}")

    validate_generated_summary(root / "result-summary.generated.md", errors)

    if args.require_runtime:
        for name in RUNTIME_FILES:
            path = root / name
            if not path.exists():
                errors.append(f"missing runtime artifact: {path}")
                continue
            content = read_text(path)
            if not content:
                errors.append(f"runtime artifact is empty: {path}")
                continue
            if name in ("cache-revision-check.txt", "option-sync-check.txt") and contains_placeholder(content):
                errors.append(f"runtime note still contains placeholder content: {path}")
        run_artifacts = sorted(phase3_dir.glob("run-*.json"))
        if not run_artifacts:
            errors.append(f"missing Phase 3 runner JSON artifacts in {phase3_dir}")

    topology_path = root / "topology.json"
    topology = {}
    if topology_path.exists():
        try:
            topology = read_json(topology_path)
        except Exception as exc:  # noqa: BLE001
            errors.append(f"invalid JSON in {topology_path}: {exc}")

    for label, filename in (
        ("old binary identity", "old-binary-identity.txt"),
        ("new binary identity", "new-binary-identity.txt"),
    ):
        path = root / filename
        if path.exists():
            value = read_text(path)
            if contains_placeholder(value):
                warnings.append(f"{label} still contains placeholder/empty value: {path}")

    if topology:
        checks = {
            "old_binary.image_or_commit": topology.get("old_binary", {}).get("image_or_commit", ""),
            "new_binary.image_or_commit": topology.get("new_binary", {}).get("image_or_commit", ""),
            "compose.file": topology.get("compose", {}).get("file", ""),
            "compose.env_file": topology.get("compose", {}).get("env_file", ""),
            "verification_targets.base_url": topology.get("verification_targets", {}).get("base_url", ""),
        }
        for field, value in checks.items():
            if contains_placeholder(str(value)):
                warnings.append(f"topology field still looks empty/placeholder: {field}")

    ok = not errors and (not warnings or not args.strict_placeholders)
    print(
        json.dumps(
            {
                "ok": ok,
                "evidence_dir": str(root),
                "errors": errors,
                "warnings": warnings,
            },
            ensure_ascii=True,
            indent=2,
        )
    )
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
