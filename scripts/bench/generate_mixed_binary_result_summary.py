#!/usr/bin/env python3
"""
Generate a draft mixed-binary result summary from the current evidence scaffold.

This helper is intentionally conservative:
- it does not claim PASS/FAIL automatically
- it reads the scaffold state and validator output
- it emits a markdown draft that an operator can complete after the real lane
"""

from __future__ import annotations

import argparse
import json
import subprocess
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent.parent
DEFAULT_EVIDENCE_DIR = REPO_ROOT / ".omx/plans/evidence/reclaim-phase0/mixed-binary"
DEFAULT_OUTPUT = DEFAULT_EVIDENCE_DIR / "result-summary.generated.md"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate a draft mixed-binary result summary from the current scaffold."
    )
    parser.add_argument(
        "--evidence-dir",
        default=str(DEFAULT_EVIDENCE_DIR),
        help=f"Mixed-binary evidence directory (default: {DEFAULT_EVIDENCE_DIR})",
    )
    parser.add_argument(
        "--output",
        default=str(DEFAULT_OUTPUT),
        help=f"Output markdown file (default: {DEFAULT_OUTPUT})",
    )
    parser.add_argument(
        "--strict-placeholders",
        action="store_true",
        help="Pass strict placeholder validation through to the validator snapshot.",
    )
    parser.add_argument(
        "--require-runtime",
        action="store_true",
        help="Pass runtime artifact requirements through to the validator snapshot.",
    )
    parser.add_argument(
        "--phase3-dir",
        default="",
        help="Optional Phase 3 evidence directory passed through to the validator.",
    )
    return parser.parse_args()


def read_text(path: Path) -> str:
    if not path.exists():
        return ""
    return path.read_text(encoding="utf-8").strip()


def read_json(path: Path) -> dict:
    if not path.exists():
        return {}
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def run_validator(evidence_dir: Path, strict_placeholders: bool, require_runtime: bool, phase3_dir: str) -> tuple[bool, dict]:
    cmd = ["python3", str(SCRIPT_DIR / "validate_mixed_binary_evidence.py"), "--dir", str(evidence_dir)]
    if strict_placeholders:
        cmd.append("--strict-placeholders")
    if require_runtime:
        cmd.append("--require-runtime")
    if phase3_dir:
        cmd.extend(["--phase3-dir", phase3_dir])

    proc = subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        check=False,
    )
    try:
        payload = json.loads(proc.stdout.strip() or "{}")
    except json.JSONDecodeError:
        payload = {"ok": False, "errors": [f"validator output was not valid JSON: {proc.stdout!r}"], "warnings": []}
    return proc.returncode == 0, payload


def main() -> int:
    args = parse_args()
    evidence_dir = Path(args.evidence_dir)
    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    if not output_path.exists():
        output_path.write_text("# Mixed-Binary External Lane Result Summary\n\nPending generation.\n", encoding="utf-8")

    old_identity = read_text(evidence_dir / "old-binary-identity.txt")
    new_identity = read_text(evidence_dir / "new-binary-identity.txt")
    topology = read_json(evidence_dir / "topology.json")
    validator_ok, validator_payload = run_validator(
        evidence_dir,
        strict_placeholders=args.strict_placeholders,
        require_runtime=args.require_runtime,
        phase3_dir=args.phase3_dir,
    )

    mysql = topology.get("shared_topology", {}).get("mysql", {})
    redis = topology.get("shared_topology", {}).get("redis", {})
    compose = topology.get("compose", {})
    warnings = validator_payload.get("warnings", [])
    errors = validator_payload.get("errors", [])

    lines = [
        "# Mixed-Binary External Lane Result Summary",
        "",
        "This is an auto-generated draft. Review and edit it before treating it as the final lane result.",
        "",
        "## Validator Snapshot",
        "",
        f"- Validator status: {'PASS' if validator_ok else 'FAIL'}",
        f"- Strict placeholders: {'on' if args.strict_placeholders else 'off'}",
        f"- Require runtime artifacts: {'on' if args.require_runtime else 'off'}",
        f"- Warning count: {len(warnings)}",
        f"- Error count: {len(errors)}",
        "",
        "## Outcome",
        "",
        "- Final status: DRAFT",
        "- Date:",
        "- Operator:",
        "",
        "## Binary Pair",
        "",
        f"- Old binary: {old_identity or '<fill-me>'}",
        f"- New binary: {new_identity or '<fill-me>'}",
        "",
        "## Shared Topology",
        "",
        f"- MySQL: {mysql.get('host', '') or '<fill-me>'}",
        f"- Redis: {redis.get('host', '') or '<fill-me>'}",
        f"- Compose file: {compose.get('file', '') or '<fill-me>'}",
        f"- Env file: {compose.get('env_file', '') or '<fill-me>'}",
        "",
        "## Validator Warnings",
        "",
    ]
    if warnings:
        for item in warnings:
            lines.append(f"- {item}")
    else:
        lines.append("- none")

    lines.extend(
        [
            "",
            "## Validator Errors",
            "",
        ]
    )
    if errors:
        for item in errors:
            lines.append(f"- {item}")
    else:
        lines.append("- none")

    lines.extend(
        [
            "",
            "## What Passed",
            "",
            "- [ ]",
            "- [ ]",
            "- [ ]",
            "",
            "## What Failed Or Remains Open",
            "",
            "- [ ]",
            "- [ ]",
            "- [ ]",
            "",
            "## Rollback Result",
            "",
            "- Rollback attempted: yes / no",
            "- Rollback outcome:",
            "- Manual intervention required: yes / no",
            "",
            "## Linked Artifacts",
            "",
            "- `old-binary-identity.txt`",
            "- `new-binary-identity.txt`",
            "- `topology.json`",
            "- `compose-ps.txt`",
            "- `compose-logs.txt`",
            "- `docker-stats.txt`",
            "- `cache-revision-check.txt`",
            "- `option-sync-check.txt`",
            "- Phase 3 runner JSONs",
            "- MySQL / Redis evidence",
            "- deviation log",
            "",
            "## Release Recommendation",
            "",
            "- Recommended next action:",
            "- Blocking issues:",
            "- Follow-up owner:",
            "",
        ]
    )

    output_path.write_text("\n".join(lines), encoding="utf-8")
    print(output_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
