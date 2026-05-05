# Upstream Merge Tracker

## Background

This repo is selectively merging newer upstream `new-api` changes into the current fork.

The goal is not to rewrite local behavior wholesale. The goal is to port specific upstream feature slices while preserving fork-specific behavior that already exists here, especially:

- current quota bucket / product snapshot billing model
- existing channel settings persistence model
- existing admin/frontend structure where it already diverged
- local runtime constraints in the current environment

## Why These Batches Exist

The upstream branch introduced several new operational slices that are not present, or only half-present, in this fork. We are merging them in must-have order so runtime behavior becomes consistent and future upstream slices can land on top safely.

The completed batches so far were chosen because they were hard prerequisites for later work:

1. request body storage core
2. overload protection / performance safeguards
3. async task timeout / settlement / public task ID separation
4. upstream model auto-detect
5. review-fix batch for the merged slices above
6. follow-up hardening for request body storage / overload protection review findings
7. upstream auto-detect parity follow-up from later upstream commits
8. async task multi-key polling parity and overload-threshold follow-up
9. origin/remix parity and reusable-body parsing follow-up

## Completed Batches

### `0ac3b9fc` `port upstream request body storage core`

Purpose:

- port upstream request body storage base logic needed by later relay/runtime changes

### `9ca1ee6c` `port upstream performance overload protection`

Purpose:

- port upstream overload/performance protection slice first because it is runtime critical

### `c2e1d85e` `port async task timeout and settlement flow`

Purpose:

- finish the half-migrated async task lifecycle
- move from old direct task handling toward upstream submit/polling/settlement flow

Main scope:

- wire task polling service into runtime
- add async task timeout sweeping and CAS updates
- separate public `task_id` from private upstream `task_id`
- expand task DTO/query output
- align async task adaptor interfaces and upstream model handling

Key files:

- [main.go](/mnt/d/code/codex-x/new-api/main.go)
- [controller/task.go](/mnt/d/code/codex-x/new-api/controller/task.go)
- [relay/relay_task.go](/mnt/d/code/codex-x/new-api/relay/relay_task.go)
- [service/task_polling.go](/mnt/d/code/codex-x/new-api/service/task_polling.go)
- [service/task_billing.go](/mnt/d/code/codex-x/new-api/service/task_billing.go)
- [model/task.go](/mnt/d/code/codex-x/new-api/model/task.go)

### `61627ba7` `fix async task payload sanitization and settlement persistence`

Purpose:

- close the review findings from the previous batch

Main scope:

- sanitize `task.Data` so upstream task IDs do not leak back out
- persist settlement result updates consistently

Key files:

- [service/task_payload.go](/mnt/d/code/codex-x/new-api/service/task_payload.go)
- [service/task_billing.go](/mnt/d/code/codex-x/new-api/service/task_billing.go)
- [service/task_polling.go](/mnt/d/code/codex-x/new-api/service/task_polling.go)
- [relay/relay_task.go](/mnt/d/code/codex-x/new-api/relay/relay_task.go)

### Review-Fix Batch

Purpose:

- review the merged upstream slices end-to-end before moving on
- close the consistency gaps discovered during post-merge review
- leave a concrete audit trail for later manual inspection

Review coverage:

- async task timeout / settlement / payload batch: reviewed by multiple subagents and fixed
- upstream model auto-detect batch: backend + frontend reviewed separately and fixed
- request body storage + overload protection batch: later re-reviewed and promoted into a follow-up hardening batch below

Findings fixed in this batch:

- task settlement rollback now compensates quota changes when task-row persistence fails
- task billing snapshots now persist `token_key` so deleted tokens do not break later refund/settlement flows
- async task submit no longer records consume logs and quota usage stats before `task.Insert()` succeeds
- task payload sanitization now replaces embedded upstream task IDs instead of only exact-match values
- channel upstream auto-detect now updates `models` and abilities in one transaction
- upstream model fetch now respects Gemini header overrides and inspects all enabled keys in multi-key mode instead of mutating polling state
- frontend upstream-update modal no longer persists unchecked additions into ignored models, and now blocks empty submit

Files touched in this batch:

- [controller/channel.go](/mnt/d/code/codex-x/new-api/controller/channel.go)
- [controller/channel_upstream_update.go](/mnt/d/code/codex-x/new-api/controller/channel_upstream_update.go)
- [model/task.go](/mnt/d/code/codex-x/new-api/model/task.go)
- [relay/relay_task.go](/mnt/d/code/codex-x/new-api/relay/relay_task.go)
- [service/task_billing.go](/mnt/d/code/codex-x/new-api/service/task_billing.go)
- [service/task_payload.go](/mnt/d/code/codex-x/new-api/service/task_payload.go)
- [web/src/hooks/channels/useChannelUpstreamUpdates.jsx](/mnt/d/code/codex-x/new-api/web/src/hooks/channels/useChannelUpstreamUpdates.jsx)
- [web/src/components/table/channels/modals/ChannelUpstreamUpdateModal.jsx](/mnt/d/code/codex-x/new-api/web/src/components/table/channels/modals/ChannelUpstreamUpdateModal.jsx)

### Request Body Storage / Overload Protection Hardening

Purpose:

- close the follow-up review findings from the request-body storage and overload-protection slices
- keep the disk-backed request-body path correct under cache cleanup and request replay
- remove the remaining overload-protection bypass on local token counting

Findings fixed in this batch:

- large requests now fall back to memory if disk-backed body storage cannot be created or completed locally, while preserving request-size limits
- pass-through replay `GetBody` now reopens disk-backed body files instead of re-materializing the whole request body into memory
- active disk-backed body files are skipped during admin cache cleanup, so file/size stats are not decremented twice while requests still hold open descriptors
- closed body-storage reuse now maps to server error instead of being reported as a bad client request
- `/v1/messages/count_tokens` now participates in `SystemPerformanceCheck`

Files touched in this batch:

- [common/body_storage.go](/mnt/d/code/codex-x/new-api/common/body_storage.go)
- [common/disk_cache.go](/mnt/d/code/codex-x/new-api/common/disk_cache.go)
- [common/gin.go](/mnt/d/code/codex-x/new-api/common/gin.go)
- [router/relay-router.go](/mnt/d/code/codex-x/new-api/router/relay-router.go)

### Regex Ignored Upstream Models Parity

Purpose:

- align the auto-detect ignored-model behavior with the later upstream follow-up
- make `upstream_model_update_ignored_models` support both exact entries and `regex:`-prefixed patterns

Findings fixed in this batch:

- pending upstream additions now honor `regex:`-prefixed ignored-model rules instead of treating them as plain literal strings
- controller coverage now includes a regex-ignored-model case
- edit-channel form copy now documents the `regex:` syntax so the stored setting matches runtime behavior

Files touched in this batch:

- [controller/channel_upstream_update.go](/mnt/d/code/codex-x/new-api/controller/channel_upstream_update.go)
- [controller/channel_upstream_update_test.go](/mnt/d/code/codex-x/new-api/controller/channel_upstream_update_test.go)
- [web/src/components/table/channels/modals/EditChannelModal.jsx](/mnt/d/code/codex-x/new-api/web/src/components/table/channels/modals/EditChannelModal.jsx)

### Frontend Upstream Update Apply Parity Refresh

Purpose:

- close the renewed review finding from the hand-ported auto-detect slice
- restore the frontend apply semantics for ignored upstream additions

Findings fixed in this batch:

- `useChannelUpstreamUpdates` once again sends unselected pending additions through `ignore_models` instead of silently dropping them
- upstream-update success feedback now reports ignored-model counts again
- the upstream-update modal once again allows empty submit so operators can ignore all pending additions in one shot

Files touched in this batch:

- [web/src/hooks/channels/useChannelUpstreamUpdates.jsx](/mnt/d/code/codex-x/new-api/web/src/hooks/channels/useChannelUpstreamUpdates.jsx)
- [web/src/components/table/channels/modals/ChannelUpstreamUpdateModal.jsx](/mnt/d/code/codex-x/new-api/web/src/components/table/channels/modals/ChannelUpstreamUpdateModal.jsx)

### Async Task Multi-Key / Threshold Follow-Up

Purpose:

- close the renewed async-task review findings discovered after the wider follow-up audit
- restore upstream parity for multi-key async task polling and origin-task submission
- align the default disk overload threshold with the later upstream follow-up

Findings fixed in this batch:

- video task polling now prefers the task-level key snapshot instead of always polling with the channel's raw key blob
- origin-task / remix submission now locks to a concrete enabled key on the origin channel instead of writing raw `channel.Key` into the request context
- performance monitor disk-threshold default now matches the upstream `95%` follow-up

Files touched in this batch:

- [service/task_polling.go](/mnt/d/code/codex-x/new-api/service/task_polling.go)
- [relay/relay_task.go](/mnt/d/code/codex-x/new-api/relay/relay_task.go)
- [setting/performance_setting/config.go](/mnt/d/code/codex-x/new-api/setting/performance_setting/config.go)

### Origin / Reusable-Body Follow-Up

Purpose:

- close the stricter re-review findings from the hand-ported async-task and body-storage slices
- restore upstream parity for origin/remix submit metadata before pricing and model mapping
- fix reusable request-body parsing so JSON and form payloads decode into the caller target correctly

Findings fixed in this batch:

- origin-task / remix submit now restores the original task model before pricing and model mapping
- remix submit now preserves inherited billing ratios from the source task instead of dropping them before persistence
- `UnmarshalBodyReusable` now decodes into the caller target directly and once again handles form-urlencoded / multipart payloads

Files touched in this batch:

- [relay/relay_task.go](/mnt/d/code/codex-x/new-api/relay/relay_task.go)
- [common/gin.go](/mnt/d/code/codex-x/new-api/common/gin.go)

## Current Review Checklist

These are the areas already changed and expected to be review targets:

- async task submit flow now returns public `task_id` while storing upstream task ID privately
- async task polling loop is wired from startup instead of remaining dead code
- timeout cleanup and CAS task updates are in place
- per-call task billing skips completion-time recalculation
- task payloads are sanitized before persistence where upstream task IDs could appear
- task quota settlement updates task row state and usage accounting consistently
- task DTO output now includes richer admin/user fields for review and querying
- task adaptor interface includes upstream billing hooks and all current task adaptors were aligned

## Completed Slice Snapshot

### Upstream Model Auto-Detect

Status:

- original slice landed in `25787354`
- post-merge consistency fixes landed in `8388f1bb`
- regex ignored-model parity follow-up landed in `0098709d`

Why this slice mattered:

- backend controller and helper logic for upstream model detection/application
- admin routes under channel APIs
- startup task wiring for periodic detection
- frontend hook/modal/actions/table entry points
- edit-channel form wiring for related settings
- optional admin notification chain if the local fork still supports it cleanly

Historical batch objective:

- merge the upstream channel-level model auto-detect slice into this fork
- keep the fork's existing channel page features intact instead of replacing the local implementation wholesale
- make the already-persisted `ChannelOtherSettings` upstream-update fields actually take effect at runtime

Historical backend review checklist:

- `FetchUpstreamModels` and edit-form `FetchModels` use the same upstream-fetch helper instead of drifting
- `/api/channel/upstream_updates/detect`
- `/api/channel/upstream_updates/detect_all`
- `/api/channel/upstream_updates/apply`
- `/api/channel/upstream_updates/apply_all`
- startup path calls the periodic upstream-update task
- periodic task respects per-channel `upstream_model_update_check_enabled`
- periodic task persists `last_check_time`, `last_detected_models`, `last_removed_models`
- auto-sync only applies pending additions when `upstream_model_update_auto_sync_enabled` is on
- channel runtime cache refresh follows local fork behavior: `BumpChannelCacheRevision()` plus `InitChannelCache()`

Historical frontend review checklist:

- channels page exposes single-channel detect/apply entry points
- channels page exposes detect-all/apply-all admin actions
- pending upstream add/remove models are visible in the table
- upstream-update modal is mounted and wired
- edit-channel modal reads/writes `upstream_model_update_*` settings
- edit-channel submit normalizes ignored models and clears stale detected models when detection is disabled

Non-goals kept deferred:

- no user notification preference / watcher broadcast parity in this batch
- no build, install, or test commands in WSL

Current implementation status:

- audit tracker created
- backend gap inventory completed
- frontend gap inventory completed
- backend detect/apply controller flow landed
- backend periodic auto-detect task landed
- backend channel model fetch helper unified for manual fetch and auto-detect
- frontend channels page modal/hook/action wiring landed
- frontend edit-channel upstream-update settings wiring landed
- post-merge multi-review pass completed for async task + upstream auto-detect slices
- post-merge fix batch landed for billing consistency, multi-key fetch semantics, and frontend apply semantics
- request-body / overload-protection hardening batch landed in `219a4524`
- regex ignored-model parity follow-up landed in `0098709d`
- latest re-review restored frontend upstream-update apply parity after a manual-port regression
- latest re-review also restored async task multi-key polling parity and the upstream overload disk-threshold default
- latest stricter re-review restored origin/remix submit parity and reusable-body decoding semantics
- notification preference / watcher broadcast parity intentionally deferred

Primary file map:

- [controller/channel.go](/mnt/d/code/codex-x/new-api/controller/channel.go)
- [controller/channel_upstream_update.go](/mnt/d/code/codex-x/new-api/controller/channel_upstream_update.go)
- [controller/channel_upstream_update_test.go](/mnt/d/code/codex-x/new-api/controller/channel_upstream_update_test.go)
- [router/api-router.go](/mnt/d/code/codex-x/new-api/router/api-router.go)
- [main.go](/mnt/d/code/codex-x/new-api/main.go)
- [web/src/hooks/channels/useChannelsData.jsx](/mnt/d/code/codex-x/new-api/web/src/hooks/channels/useChannelsData.jsx)
- [web/src/hooks/channels/useChannelUpstreamUpdates.jsx](/mnt/d/code/codex-x/new-api/web/src/hooks/channels/useChannelUpstreamUpdates.jsx)
- [web/src/hooks/channels/upstreamUpdateUtils.js](/mnt/d/code/codex-x/new-api/web/src/hooks/channels/upstreamUpdateUtils.js)
- [web/src/components/table/channels/index.jsx](/mnt/d/code/codex-x/new-api/web/src/components/table/channels/index.jsx)
- [web/src/components/table/channels/ChannelsTable.jsx](/mnt/d/code/codex-x/new-api/web/src/components/table/channels/ChannelsTable.jsx)
- [web/src/components/table/channels/ChannelsActions.jsx](/mnt/d/code/codex-x/new-api/web/src/components/table/channels/ChannelsActions.jsx)
- [web/src/components/table/channels/ChannelsColumnDefs.jsx](/mnt/d/code/codex-x/new-api/web/src/components/table/channels/ChannelsColumnDefs.jsx)
- [web/src/components/table/channels/modals/EditChannelModal.jsx](/mnt/d/code/codex-x/new-api/web/src/components/table/channels/modals/EditChannelModal.jsx)
- [web/src/components/table/channels/modals/ChannelUpstreamUpdateModal.jsx](/mnt/d/code/codex-x/new-api/web/src/components/table/channels/modals/ChannelUpstreamUpdateModal.jsx)
- [web/src/constants/channel.constants.js](/mnt/d/code/codex-x/new-api/web/src/constants/channel.constants.js)

## Notes

- No build/install/test commands are run in WSL for these batches.
- Each completed modification batch must be committed separately.
- Review is performed before commit, and only files belonging to the current batch should be included in the commit.
