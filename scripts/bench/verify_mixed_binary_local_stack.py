#!/usr/bin/env python3
"""
Run the local Phase 5 mixed-binary handoff verification stack.

This wrapper is intentionally repo-local and stdlib-only. It focuses on the
local scaffold/tooling layer, not the real external MySQL + Redis lane.
"""

from __future__ import annotations

import argparse
import json
import shlex
import subprocess
import sys
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent.parent


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run the local mixed-binary handoff verification stack."
    )
    parser.add_argument(
        "--output",
        default="",
        help="Optional JSON output path for the verification report.",
    )
    parser.add_argument(
        "--skip-backend-tests",
        action="store_true",
        help="Skip `go test ./model ./service ./relay/helper ./middleware ./controller/...`.",
    )
    parser.add_argument(
        "--skip-relay-tests",
        action="store_true",
        help="Skip `go test ./relay/...`.",
    )
    parser.add_argument(
        "--skip-frontend-prettier",
        action="store_true",
        help="Skip the affected-file prettier check used by the current reclaim UI slice.",
    )
    parser.add_argument(
        "--skip-preflight-shell-wrapper",
        action="store_true",
        help="Skip the cwd-safe preflight shell wrapper step (used to avoid recursion when the shell wrapper invokes this script).",
    )
    return parser.parse_args()


def run_step(name: str, cmd: list[str], cwd: Path, expect_failure: bool = False) -> dict:
    proc = subprocess.run(cmd, cwd=str(cwd), capture_output=True, text=True)
    ok = proc.returncode == 0
    if expect_failure:
        ok = proc.returncode != 0
    return {
        "name": name,
        "ok": ok,
        "returncode": proc.returncode,
        "expect_failure": expect_failure,
        "stdout": proc.stdout.strip(),
        "stderr": proc.stderr.strip(),
        "cmd": cmd,
    }


def bash_step(name: str, script: str, expect_failure: bool = False) -> dict:
    return run_step(name, ["bash", "-lc", script], REPO_ROOT, expect_failure=expect_failure)


def main() -> int:
    args = parse_args()
    repo_root = shlex.quote(str(REPO_ROOT))
    preflight_script = shlex.quote(str(REPO_ROOT / ".omx/plans/evidence/reclaim-phase0/mixed-binary/preflight-local.template.sh"))
    prepare_script = shlex.quote(str(REPO_ROOT / "scripts/bench/prepare_mixed_binary_evidence.py"))
    generate_script = shlex.quote(str(REPO_ROOT / "scripts/bench/generate_mixed_binary_result_summary.py"))
    validate_script = shlex.quote(str(REPO_ROOT / "scripts/bench/validate_mixed_binary_evidence.py"))
    commands_script = shlex.quote(str(REPO_ROOT / ".omx/plans/evidence/reclaim-phase0/mixed-binary/commands.template.sh"))
    capture_script = shlex.quote(str(REPO_ROOT / ".omx/plans/evidence/reclaim-phase0/mixed-binary/capture-runtime.template.sh"))
    archive_script = shlex.quote(str(REPO_ROOT / ".omx/plans/evidence/reclaim-phase0/mixed-binary/archive.template.sh"))
    finalize_script = shlex.quote(str(REPO_ROOT / ".omx/plans/evidence/reclaim-phase0/mixed-binary/finalize-evidence.template.sh"))

    steps = [
        run_step(
            "py_compile_helpers",
            [
                "python3",
                "-m",
                "py_compile",
                "scripts/bench/prepare_mixed_binary_evidence.py",
                "scripts/bench/validate_mixed_binary_evidence.py",
                "scripts/bench/generate_mixed_binary_result_summary.py",
                "scripts/bench/lint_mixed_binary_templates.py",
                "scripts/bench/verify_mixed_binary_local_stack.py",
            ],
            REPO_ROOT,
        ),
        run_step(
            "shell_template_syntax",
            [
                "bash",
                "-n",
                ".omx/plans/evidence/reclaim-phase0/mixed-binary/commands.template.sh",
                ".omx/plans/evidence/reclaim-phase0/mixed-binary/preflight-local.template.sh",
                ".omx/plans/evidence/reclaim-phase0/mixed-binary/capture-runtime.template.sh",
                ".omx/plans/evidence/reclaim-phase0/mixed-binary/archive.template.sh",
                ".omx/plans/evidence/reclaim-phase0/mixed-binary/finalize-evidence.template.sh",
            ],
            REPO_ROOT,
        ),
        run_step(
            "doc_lint",
            ["python3", "scripts/bench/lint_mixed_binary_templates.py"],
            REPO_ROOT,
        ),
        run_step(
            "validator_default",
            ["python3", "scripts/bench/validate_mixed_binary_evidence.py"],
            REPO_ROOT,
        ),
        run_step(
            "validator_strict_placeholders",
            ["python3", "scripts/bench/validate_mixed_binary_evidence.py", "--strict-placeholders"],
            REPO_ROOT,
            expect_failure=True,
        ),
        run_step(
            "validator_strict_runtime",
            [
                "python3",
                "scripts/bench/validate_mixed_binary_evidence.py",
                "--strict-placeholders",
                "--require-runtime",
            ],
            REPO_ROOT,
            expect_failure=True,
        ),
        run_step(
            "strict_runtime_summary",
            [
                "python3",
                "scripts/bench/generate_mixed_binary_result_summary.py",
                "--strict-placeholders",
                "--require-runtime",
            ],
            REPO_ROOT,
        ),
        bash_step(
            "temp_scaffold_roundtrip",
            r'''
tmpmix=$(mktemp -d)
trap 'rm -rf "$tmpmix"' EXIT
cd /tmp
python3 __PREPARE_SCRIPT__ \
  --old-binary old-commit-placeholder \
  --new-binary new-commit-placeholder \
  --compose-file docker-compose.yml \
  --env-file data/.env \
  --base-url http://127.0.0.1:3000 \
  --output-dir "$tmpmix" \
  --force >/dev/null
python3 __GENERATE_SCRIPT__ \
  --evidence-dir "$tmpmix" \
  --output "$tmpmix/result-summary.generated.md" >/dev/null
python3 __VALIDATE_SCRIPT__ \
  --dir "$tmpmix" >/dev/null
test -f "$tmpmix/result-summary.generated.md"
'''.replace("__PREPARE_SCRIPT__", prepare_script)
   .replace("__GENERATE_SCRIPT__", generate_script)
   .replace("__VALIDATE_SCRIPT__", validate_script),
        ),
        bash_step(
            "default_cwd_independent_helpers",
            r'''
cd /tmp
python3 __PREPARE_SCRIPT__ \
  --old-binary old-commit-placeholder \
  --new-binary new-commit-placeholder \
  --compose-file docker-compose.yml \
  --env-file data/.env \
  --base-url http://127.0.0.1:3000 \
  --force >/dev/null
python3 __GENERATE_SCRIPT__ >/dev/null
python3 __VALIDATE_SCRIPT__ >/dev/null
'''.replace("__PREPARE_SCRIPT__", prepare_script)
   .replace("__GENERATE_SCRIPT__", generate_script)
   .replace("__VALIDATE_SCRIPT__", validate_script),
        ),
    ]

    if not args.skip_backend_tests:
        steps.append(
            run_step(
                "backend_tests",
                ["go", "test", "./model", "./service", "./relay/helper", "./middleware", "./controller/..."],
                REPO_ROOT,
            )
        )

    if not args.skip_preflight_shell_wrapper:
        steps.append(
            bash_step(
                "preflight_local_shell_wrapper",
                r'''
tmpjson=$(mktemp)
trap 'rm -f "$tmpjson"' EXIT
cd /tmp
ROOT=__REPO_ROOT__ \
  bash __PREFLIGHT_SCRIPT__ > "$tmpjson"
python3 - <<'PY' "$tmpjson"
import json, sys
from pathlib import Path
p = Path(sys.argv[1])
data = json.loads(p.read_text())
assert data.get("ok") is True, data
assert len(data.get("steps", [])) >= 1, data
PY
'''.replace("__REPO_ROOT__", repo_root)
   .replace("__PREFLIGHT_SCRIPT__", preflight_script),
            )
        )

    steps.extend([
        bash_step(
            "capture_runtime_smoke",
            r'''
tmpdir=$(mktemp -d)
stubdir=$(mktemp -d)
trap 'rm -rf "$tmpdir" "$stubdir"' EXIT
cat > "$stubdir/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "compose" ]]; then
  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -f|--env-file)
        shift 2
        ;;
      ps)
        printf 'service\n'
        exit 0
        ;;
      logs)
        printf 'logs\n'
        exit 0
        ;;
      *)
        shift
        ;;
    esac
  done
  printf 'unexpected docker compose args: %s\n' "$*" >&2
  exit 1
fi
if [[ "$1" == "stats" && "${2:-}" == "--no-stream" ]]; then
  printf 'stats\n'
  exit 0
fi
printf 'unexpected docker args: %s\n' "$*" >&2
exit 1
SH
chmod +x "$stubdir/docker"
PATH="$stubdir:$PATH" ROOT=__REPO_ROOT__ OUT_DIR="$tmpdir" DOCKER_BIN=docker COMPOSE_FILE=custom-compose.yml ENV_FILE=custom.env \
  bash __CAPTURE_SCRIPT__ >/dev/null
test -f "$tmpdir/compose-ps.txt"
test -f "$tmpdir/compose-logs.txt"
test -f "$tmpdir/docker-stats.txt"
test -f "$tmpdir/cache-revision-check.template.md"
test -f "$tmpdir/option-sync-check.template.md"
test -f "$tmpdir/cache-revision-check.txt"
test -f "$tmpdir/option-sync-check.txt"
'''.replace("__REPO_ROOT__", repo_root)
   .replace("__CAPTURE_SCRIPT__", capture_script),
        ),
        bash_step(
            "commands_template_smoke",
            r'''
tmpmix=$(mktemp -d)
tmpp3=$(mktemp -d)
stubdir=$(mktemp -d)
trap 'rm -rf "$tmpmix" "$tmpp3" "$stubdir"' EXIT
cat > "$stubdir/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "compose" ]]; then
  shift
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -f|--env-file)
        shift 2
        ;;
      ps)
        printf 'service\n'
        exit 0
        ;;
      logs)
        printf 'logs\n'
        exit 0
        ;;
      *)
        shift
        ;;
    esac
  done
  printf 'unexpected docker compose args: %s\n' "$*" >&2
  exit 1
fi
if [[ "$1" == "stats" && "${2:-}" == "--no-stream" ]]; then
  printf 'stats\n'
  exit 0
fi
printf 'unexpected docker args: %s\n' "$*" >&2
exit 1
SH
chmod +x "$stubdir/docker"
cd /tmp
PATH="$stubdir:$PATH" ROOT=__REPO_ROOT__ EVIDENCE_DIR="$tmpmix" PHASE3_DIR="$tmpp3" DOCKER_BIN=docker \
  OLD_ID=old-real NEW_ID=new-real OLD_CONTAINER=old-app NEW_CONTAINER=new-app COMPOSE_FILE=docker-compose.yml ENV_FILE=data/.env MYSQL_HOST=mysql.internal MYSQL_DATABASE=oneapi REDIS_HOST=redis.internal REDIS_DATABASE=0 BASE_URL=http://127.0.0.1:9 DATASET_NOTE=test-note STARTUP_CLEANUP_LEGACY_OPTIONS_ENABLED=true \
  bash __COMMANDS_SCRIPT__ >/dev/null
ls "$tmpp3"/run-*.json >/dev/null
python3 - <<'PY' "$tmpmix/topology.json"
import json, sys
from pathlib import Path
p = Path(sys.argv[1])
data = json.loads(p.read_text())
assert data["old_binary"]["container_name"] == "old-app", data
assert data["new_binary"]["container_name"] == "new-app", data
assert data["shared_topology"]["mysql"]["host"] == "mysql.internal", data
assert data["shared_topology"]["mysql"]["database"] == "oneapi", data
assert data["shared_topology"]["redis"]["host"] == "redis.internal", data
assert data["shared_topology"]["redis"]["database"] == "0", data
assert data["startup_cleanup"]["env_STARTUP_CLEANUP_LEGACY_OPTIONS_ENABLED"] is True, data
assert data["startup_cleanup"]["db_option_startup_cleanup_legacy_options_enabled"] is True, data
PY
'''.replace("__REPO_ROOT__", repo_root)
   .replace("__COMMANDS_SCRIPT__", commands_script),
        ),
        bash_step(
            "archive_template_guard",
            r'''
cd /tmp
ROOT=__REPO_ROOT__ \
  bash __ARCHIVE_SCRIPT__
'''.replace("__REPO_ROOT__", repo_root)
   .replace("__ARCHIVE_SCRIPT__", archive_script),
            expect_failure=True,
        ),
        bash_step(
            "archive_template_smoke",
            r'''
tmpmix=$(mktemp -d)
tmpp3=$(mktemp -d)
tmpout=$(mktemp -d)
trap 'rm -rf "$tmpmix" "$tmpp3" "$tmpout"' EXIT
python3 __PREPARE_SCRIPT__ \
  --old-binary old-commit-real \
  --new-binary new-commit-real \
  --compose-file docker-compose.yml \
  --env-file data/.env \
  --mysql-host mysql.internal \
  --mysql-database oneapi \
  --redis-host redis.internal \
  --redis-database 0 \
  --base-url http://127.0.0.1:3000 \
  --output-dir "$tmpmix" \
  --force >/dev/null
printf 'NAME STATE\nold running\nnew running\n' > "$tmpmix/compose-ps.txt"
printf 'service log line\n' > "$tmpmix/compose-logs.txt"
printf 'CONTAINER CPU MEM\nold 1%% 10MiB\nnew 1%% 11MiB\n' > "$tmpmix/docker-stats.txt"
cat > "$tmpmix/cache-revision-check.txt" <<'EOF'
Before Change
- Old binary observed revision: 100
- New binary observed revision: 100
After Change
- Old binary observed revision: 101
- New binary observed revision: 101
Outcome
- Both binaries observed the new revision: yes
EOF
cat > "$tmpmix/option-sync-check.txt" <<'EOF'
Context
- Option key(s) under observation: channel_cache.revision
After Change
- Old binary observed value: 101
- New binary observed value: 101
Outcome
- Option sync remained coherent across binaries: yes
EOF
printf '{"generated_at_utc":"2026-04-20T00:00:00Z","label":"mixed-binary-soak","results":[],"summary":{"total":0,"success":0,"failure":0,"elapsed_total_ms":0}}\n' > "$tmpp3/run-dummy.json"
ROOT=__REPO_ROOT__ MIXED_DIR="$tmpmix" PHASE3_DIR="$tmpp3" OUT_DIR="$tmpout" \
  bash __ARCHIVE_SCRIPT__ >/tmp/archive-smoke-out.txt
find "$tmpout" -maxdepth 1 -name 'mixed-binary-evidence-*.tar.gz' -print -quit | grep -q .
'''.replace("__REPO_ROOT__", repo_root)
   .replace("__PREPARE_SCRIPT__", prepare_script)
   .replace("__ARCHIVE_SCRIPT__", archive_script),
        ),
        bash_step(
            "finalize_template_guard",
            r'''
cd /tmp
ROOT=__REPO_ROOT__ \
  bash __FINALIZE_SCRIPT__
'''.replace("__REPO_ROOT__", repo_root)
   .replace("__FINALIZE_SCRIPT__", finalize_script),
            expect_failure=True,
        ),
        bash_step(
            "finalize_template_smoke",
            r'''
tmpmix=$(mktemp -d)
tmpp3=$(mktemp -d)
tmpout=$(mktemp -d)
trap 'rm -rf "$tmpmix" "$tmpp3" "$tmpout"' EXIT
python3 __PREPARE_SCRIPT__ \
  --old-binary old-commit-real \
  --new-binary new-commit-real \
  --compose-file docker-compose.yml \
  --env-file data/.env \
  --mysql-host mysql.internal \
  --mysql-database oneapi \
  --redis-host redis.internal \
  --redis-database 0 \
  --base-url http://127.0.0.1:3000 \
  --output-dir "$tmpmix" \
  --force >/dev/null
printf 'NAME STATE\nold running\nnew running\n' > "$tmpmix/compose-ps.txt"
printf 'service log line\n' > "$tmpmix/compose-logs.txt"
printf 'CONTAINER CPU MEM\nold 1%% 10MiB\nnew 1%% 11MiB\n' > "$tmpmix/docker-stats.txt"
cat > "$tmpmix/cache-revision-check.txt" <<'EOF'
Before Change
- Old binary observed revision: 100
- New binary observed revision: 100
After Change
- Old binary observed revision: 101
- New binary observed revision: 101
Outcome
- Both binaries observed the new revision: yes
EOF
cat > "$tmpmix/option-sync-check.txt" <<'EOF'
Context
- Option key(s) under observation: channel_cache.revision
After Change
- Old binary observed value: 101
- New binary observed value: 101
Outcome
- Option sync remained coherent across binaries: yes
EOF
printf '{"generated_at_utc":"2026-04-20T00:00:00Z","label":"mixed-binary-soak","results":[],"summary":{"total":0,"success":0,"failure":0,"elapsed_total_ms":0}}\n' > "$tmpp3/run-dummy.json"
ROOT=__REPO_ROOT__ MIXED_DIR="$tmpmix" PHASE3_DIR="$tmpp3" OUT_DIR="$tmpout" RESULT_SUMMARY_OUTPUT="$tmpmix/result-summary.generated.md" \
  bash __FINALIZE_SCRIPT__ >/tmp/finalize-smoke-out.txt
grep -q 'Validator status: PASS' "$tmpmix/result-summary.generated.md"
grep -q 'Strict placeholders: on' "$tmpmix/result-summary.generated.md"
grep -q 'Require runtime artifacts: on' "$tmpmix/result-summary.generated.md"
find "$tmpout" -maxdepth 1 -name 'mixed-binary-evidence-*.tar.gz' -print -quit | grep -q .
'''.replace("__REPO_ROOT__", repo_root)
   .replace("__PREPARE_SCRIPT__", prepare_script)
   .replace("__FINALIZE_SCRIPT__", finalize_script),
        ),
        run_step(
            "diff_hygiene",
            ["git", "diff", "--check"],
            REPO_ROOT,
        ),
    ])

    if not args.skip_relay_tests:
        steps.append(
            run_step("relay_tests", ["go", "test", "./relay/..."], REPO_ROOT)
        )

    if not args.skip_frontend_prettier:
        steps.append(
            run_step(
                "frontend_prettier",
                [
                    "bunx",
                    "prettier",
                    "src/components/table/users/UsersColumnDefs.jsx",
                    "src/components/table/users/modals/AddUserModal.jsx",
                    "src/components/table/users/modals/EditUserModal.jsx",
                    "src/helpers/render.jsx",
                    "src/hooks/dashboard/useDashboardData.js",
                    "src/hooks/usage-logs/useUsageLogsData.jsx",
                    "src/i18n/locales/en.json",
                    "src/i18n/locales/zh.json",
                    "src/pages/Setting/Ratio/CustomerPricingSettings.jsx",
                    "--check",
                ],
                REPO_ROOT / "web",
            )
        )

    ok = all(step["ok"] for step in steps)
    payload = {
        "ok": ok,
        "steps": steps,
    }
    if args.output:
        output_path = Path(args.output)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(json.dumps(payload, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(payload, ensure_ascii=True, indent=2))
    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
