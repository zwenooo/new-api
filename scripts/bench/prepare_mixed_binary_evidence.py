#!/usr/bin/env python3
"""
Prepare the mixed-binary evidence scaffold for the external MySQL + Redis rollout lane.

This helper writes:
- old-binary-identity.txt
- new-binary-identity.txt
- topology.json

It is intentionally stdlib-only so operators can run it in constrained environments.
"""

from __future__ import annotations

import argparse
import json
import subprocess
from pathlib import Path
from datetime import datetime, timezone
import shutil


SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent.parent
SCAFFOLD_SOURCE_DIR = REPO_ROOT / ".omx/plans/evidence/reclaim-phase0/mixed-binary"
DEFAULT_OUTPUT_DIR = REPO_ROOT / ".omx/plans/evidence/reclaim-phase0/mixed-binary"
DEFAULT_BASE_URL = "http://127.0.0.1:3000"
SCAFFOLD_FILES = [
    "README.md",
    "commands.template.sh",
    "preflight-local.template.sh",
    "capture-runtime.template.sh",
    "archive.template.sh",
    "finalize-evidence.template.sh",
    "checklist.md",
    "artifact-manifest.template.md",
    "deviation-log.template.md",
    "cache-revision-check.template.md",
    "option-sync-check.template.md",
    "result-summary.template.md",
    "topology.template.json",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Populate mixed-binary evidence scaffold files for the external rollout lane."
    )
    parser.add_argument("--old-binary", required=True, help="Old binary image digest or commit")
    parser.add_argument("--new-binary", required=True, help="New binary image digest or commit")
    parser.add_argument("--compose-file", default="", help="Compose file path used by the lane")
    parser.add_argument("--env-file", default="", help="Env file path used by the lane")
    parser.add_argument("--old-container", default="", help="Old-binary container name")
    parser.add_argument("--new-container", default="", help="New-binary container name")
    parser.add_argument("--mysql-host", default="", help="Shared MySQL host")
    parser.add_argument("--mysql-database", default="", help="Shared MySQL database")
    parser.add_argument("--redis-host", default="", help="Shared Redis host")
    parser.add_argument("--redis-database", default="", help="Shared Redis database index/name")
    parser.add_argument("--base-url", default=DEFAULT_BASE_URL, help="Base URL stored in topology.json")
    parser.add_argument(
        "--startup-cleanup-legacy-options-enabled",
        action="store_true",
        help="Set the startup cleanup flag to true in topology.json (normally keep this off)",
    )
    parser.add_argument(
        "--output-dir",
        default=str(DEFAULT_OUTPUT_DIR),
        help=f"Output directory for evidence scaffold files (default: {DEFAULT_OUTPUT_DIR})",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Accepted for compatibility with the documented workflow; existing scaffold files are overwritten by default.",
    )
    return parser.parse_args()


def write_text(path: Path, value: str) -> None:
    path.write_text(value.rstrip() + "\n", encoding="utf-8")


def read_json(path: Path) -> dict:
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def utc_now_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def ensure_scaffold_files(output_dir: Path) -> None:
    for name in SCAFFOLD_FILES:
        src = SCAFFOLD_SOURCE_DIR / name
        dst = output_dir / name
        if dst.exists():
            continue
        if not src.exists():
            raise SystemExit(f"missing scaffold source file: {src}")
        shutil.copy2(src, dst)


def main() -> None:
    args = parse_args()
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    ensure_scaffold_files(output_dir)
    template_path = output_dir / "topology.template.json"
    if not template_path.exists():
        raise SystemExit(f"missing topology template: {template_path}")

    write_text(output_dir / "old-binary-identity.txt", args.old_binary)
    write_text(output_dir / "new-binary-identity.txt", args.new_binary)

    topology = read_json(template_path)
    topology.setdefault("old_binary", {})
    topology["old_binary"]["image_or_commit"] = args.old_binary
    topology["old_binary"]["container_name"] = args.old_container

    topology.setdefault("new_binary", {})
    topology["new_binary"]["image_or_commit"] = args.new_binary
    topology["new_binary"]["container_name"] = args.new_container

    topology.setdefault("shared_topology", {})
    topology["shared_topology"].setdefault("mysql", {})
    topology["shared_topology"]["mysql"]["host"] = args.mysql_host
    topology["shared_topology"]["mysql"]["database"] = args.mysql_database
    topology["shared_topology"].setdefault("redis", {})
    topology["shared_topology"]["redis"]["host"] = args.redis_host
    topology["shared_topology"]["redis"]["database"] = args.redis_database

    topology.setdefault("compose", {})
    topology["compose"]["file"] = args.compose_file
    topology["compose"]["env_file"] = args.env_file

    topology.setdefault("startup_cleanup", {})
    topology["startup_cleanup"]["env_STARTUP_CLEANUP_LEGACY_OPTIONS_ENABLED"] = args.startup_cleanup_legacy_options_enabled
    topology["startup_cleanup"]["db_option_startup_cleanup_legacy_options_enabled"] = args.startup_cleanup_legacy_options_enabled

    topology["captured_at_utc"] = utc_now_iso()
    topology.setdefault("verification_targets", {})
    topology["verification_targets"]["base_url"] = args.base_url

    topology_path = output_dir / "topology.json"
    topology_path.write_text(json.dumps(topology, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    summary_output = output_dir / "result-summary.generated.md"
    summary_proc = subprocess.run(
        [
            "python3",
            str(SCRIPT_DIR / "generate_mixed_binary_result_summary.py"),
            "--evidence-dir",
            str(output_dir),
            "--output",
            str(summary_output),
        ],
        capture_output=True,
        text=True,
        check=True,
    )
    generated_summary_path = summary_proc.stdout.strip().splitlines()[-1] if summary_proc.stdout.strip() else str(summary_output)

    print(output_dir)
    print(output_dir / "old-binary-identity.txt")
    print(output_dir / "new-binary-identity.txt")
    print(topology_path)
    print(generated_summary_path)


if __name__ == "__main__":
    main()
