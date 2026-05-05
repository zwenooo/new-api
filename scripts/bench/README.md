# Reclaim Benchmark Runner

This directory contains the fixed benchmark contract scaffolding for reclaim Phase 0.

The goal is not to prove performance yet. The goal is to freeze:

- the request corpus
- the runner entrypoint
- the output artifact schema
- the evidence paths used by later review

## Files

- `reclaim_phase0_corpus.json`
  - fixed request families agreed in the PRD/test spec
- `reclaim_phase_runner.py`
  - repo-local runner that reads the fixed corpus and emits structured JSON
- `prepare_mixed_binary_evidence.py`
  - repo-local helper that initializes `.omx/plans/evidence/reclaim-phase0/mixed-binary/`
  - defaults now resolve against the repo root, not the caller's cwd
  - populates the external mixed-binary evidence scaffold files (`old-binary-identity.txt`, `new-binary-identity.txt`, `topology.json`)
  - can bootstrap the full mixed-binary scaffold into an alternate output directory
  - copies/fills `topology.json` from the checked-in template
  - can also write old/new binary identity files for the external rollout lane
  - regenerates `result-summary.generated.md` so a fresh scaffold matches the validator’s expected surface
  - invokes sibling helpers via paths resolved from its own script location
- `validate_mixed_binary_evidence.py`
  - repo-local checker for the mixed-binary scaffold directory
  - default evidence paths now resolve against the repo root, not the caller's cwd
  - verifies the current mixed-binary scaffold file set and JSON structure, including the checked-in topology/cache/option templates
  - reports placeholder / empty-field warnings and can fail on them with `--strict-placeholders`
  - can also require runtime capture artifacts, cache/option sync notes, and Phase 3 runner JSONs via `--require-runtime`
- `generate_mixed_binary_result_summary.py`
  - emits a markdown draft result summary from the current mixed-binary scaffold
  - default evidence paths now resolve against the repo root, not the caller's cwd
  - invokes the validator via a path resolved from its own script location
  - captures validator status, warnings, and current topology/identity values
  - can mirror the final-gate validator mode via `--strict-placeholders --require-runtime`
  - does not claim PASS/FAIL automatically; it prepares an operator-editable draft
- `lint_mixed_binary_templates.py`
  - checks the mixed-binary README / handoff / phase5 status / status index for artifact-list drift
  - provides a local consistency check for the Phase 5 handoff docs layer
- `mixed_binary_scaffold_manifest.json`
  - single source of truth for the mixed-binary checked-in scaffold file list and the doc-lint artifact list
- `verify_mixed_binary_local_stack.py`
  - wraps the current local Phase 5 handoff verification stack into one repo-local command
  - by default also includes `go test ./relay/...` and the affected-file frontend Prettier check
  - checks Python helper syntax, shell template syntax, doc consistency, validator behavior, generated-summary behavior, default cwd-independent helper invocation, the cwd-safe preflight shell wrapper, fresh-OUT_DIR runtime capture seeding, commands-template topology propagation, positive archive/finalize alternate-dir smokes, and diff hygiene
  - supports `--skip-relay-tests`, `--skip-frontend-prettier`, and `--skip-preflight-shell-wrapper` for narrower local reruns

## Usage

```bash
cd /mnt/d/code/codex-x/new-api
python3 scripts/bench/reclaim_phase_runner.py \
  --label current-new-api \
  --base-url http://127.0.0.1:3000 \
  --output-dir .omx/plans/evidence/reclaim-phase3 \
  --dataset-note "local smoke dataset"
```

Initialize the external mixed-binary evidence directory:

```bash
cd /mnt/d/code/codex-x/new-api
python3 scripts/bench/prepare_mixed_binary_evidence.py \
  --old-binary '<old-image-or-commit>' \
  --new-binary '<new-image-or-commit>' \
  --compose-file docker-compose.yml \
  --env-file data/.env
```

Validate the scaffold after capture:

```bash
cd /mnt/d/code/codex-x/new-api
python3 scripts/bench/validate_mixed_binary_evidence.py
```

Validate the final external-lane evidence set:

```bash
cd /mnt/d/code/codex-x/new-api
python3 scripts/bench/validate_mixed_binary_evidence.py --strict-placeholders --require-runtime
```

Generate a draft result summary from the current scaffold:

```bash
cd /mnt/d/code/codex-x/new-api
python3 scripts/bench/generate_mixed_binary_result_summary.py
```

Generate a draft result summary using the same strict runtime mode as final signoff:

```bash
cd /mnt/d/code/codex-x/new-api
python3 scripts/bench/generate_mixed_binary_result_summary.py --strict-placeholders --require-runtime
```

Lint the mixed-binary handoff docs for artifact-list consistency:

```bash
cd /mnt/d/code/codex-x/new-api
python3 scripts/bench/lint_mixed_binary_templates.py
```

Run the whole local mixed-binary handoff verification stack:

```bash
cd /mnt/d/code/codex-x/new-api
python3 scripts/bench/verify_mixed_binary_local_stack.py
```

Run a narrower local verification pass:

```bash
cd /mnt/d/code/codex-x/new-api
python3 scripts/bench/verify_mixed_binary_local_stack.py --skip-relay-tests --skip-frontend-prettier
```

Write the local preflight result to a JSON file:

```bash
cd /mnt/d/code/codex-x/new-api
python3 scripts/bench/verify_mixed_binary_local_stack.py \
  --output .omx/plans/evidence/reclaim-phase0/mixed-binary/local-preflight.generated.json
```

Run the cwd-safe shell preflight entrypoint from the scaffold:

```bash
cd /mnt/d/code/codex-x/new-api
bash .omx/plans/evidence/reclaim-phase0/mixed-binary/preflight-local.template.sh
```

## Output Contract

The runner writes one JSON file per run:

```text
.omx/plans/evidence/reclaim-phase3/run-<utc-ts>-<label>.json
```

Each artifact records:

- UTC timestamp
- label / binary identity
- base URL
- dataset note
- corpus file and corpus version
- one result entry per corpus item
- aggregate counts for total, success, failure
- elapsed wall time in milliseconds

## Notes

- This runner is intentionally dependency-free and uses Python standard library only.
- It is safe to use for local or staging baselines.
- Production-like approval still requires the MySQL-backed benchmark lane described in:
  - [benchmark-contract.md](/mnt/d/code/codex-x/new-api/.omx/plans/evidence/reclaim-phase0/benchmark-contract.md)
  - [mixed-binary-lane-procedure.md](/mnt/d/code/codex-x/new-api/.omx/plans/evidence/reclaim-phase0/mixed-binary-lane-procedure.md)
  - [reclaim-phase5-rollout-handoff.md](/mnt/d/code/codex-x/new-api/.omx/plans/evidence/reclaim-phase5-rollout-handoff.md)
