#!/bin/sh
set -eu

usage() {
  cat <<'EOF'
Usage:
  ./health-watch.sh

Continuously scans Docker for unhealthy containers and restarts the
containers that belong to the current compose project and are explicitly
labelled for auto-restart. The container must stay unhealthy for a grace
period before restart, and repeated failures back off automatically.

Env:
  WATCH_PROJECT=                       # optional compose project name override
  WATCH_CONTAINERS="new-api redis mysql"
                                       # fallback list when compose-project discovery is unavailable
  WATCH_LABEL_KEY=io.codexx.health-watch.restart
                                       # label key required in compose-project mode
  WATCH_LABEL_VALUE=true               # label value required in compose-project mode
  CHECK_INTERVAL=10                    # seconds between scans
  LOG_DIR=/var/log/docker-health       # directory for unhealthy snapshots
  LOG_TAIL=200                         # number of container log lines to persist before restart
  UNHEALTHY_GRACE_PERIOD=60            # unhealthy duration required before restart
  RESTART_BASE_COOLDOWN=120            # cooldown after the first restart attempt
  RESTART_MAX_COOLDOWN=1800            # upper bound for exponential backoff
  STATE_DIR=/var/log/docker-health/.state
                                       # directory for watcher state
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

WATCH_PROJECT="${WATCH_PROJECT:-}"
WATCH_CONTAINERS="${WATCH_CONTAINERS:-new-api redis mysql}"
WATCH_LABEL_KEY="${WATCH_LABEL_KEY:-io.codexx.health-watch.restart}"
WATCH_LABEL_VALUE="${WATCH_LABEL_VALUE:-true}"
CHECK_INTERVAL="${CHECK_INTERVAL:-10}"
LOG_DIR="${LOG_DIR:-/var/log/docker-health}"
LOG_TAIL="${LOG_TAIL:-200}"
UNHEALTHY_GRACE_PERIOD="${UNHEALTHY_GRACE_PERIOD:-60}"
RESTART_BASE_COOLDOWN="${RESTART_BASE_COOLDOWN:-120}"
RESTART_MAX_COOLDOWN="${RESTART_MAX_COOLDOWN:-1800}"
STATE_DIR="${STATE_DIR:-$LOG_DIR/.state}"
PROJECT_LABEL_KEY="com.docker.compose.project"

log() {
  ts="$(date '+%Y-%m-%dT%H:%M:%S%z' 2>/dev/null || date)"
  printf '[%s] %s\n' "$ts" "$*" >&2
}

require_cmd() {
  cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Required command not found: $cmd" >&2
    exit 1
  fi
}

require_non_negative_int() {
  value="$1"
  name="$2"
  case "$value" in
    ''|*[!0-9]*)
      echo "Invalid $name: $value (expected non-negative integer)" >&2
      exit 1
      ;;
  esac
}

require_positive_int() {
  value="$1"
  name="$2"
  require_non_negative_int "$value" "$name"
  if [ "$value" -le 0 ]; then
    echo "Invalid $name: $value (expected positive integer)" >&2
    exit 1
  fi
}

strip_no_value() {
  value="$1"
  if [ "$value" = "<no value>" ]; then
    printf '\n'
    return 0
  fi
  printf '%s\n' "$value"
}

state_file_path() {
  container_name="$1"
  printf '%s/%s.state\n' "$STATE_DIR" "$container_name"
}

load_state() {
  container_name="$1"
  state_file="$(state_file_path "$container_name")"
  unhealthy_since=0
  restart_count=0
  cooldown_until=0

  if [ ! -f "$state_file" ]; then
    return 0
  fi

  while IFS='=' read -r key value; do
    case "$value" in
      ''|*[!0-9]*) value=0 ;;
    esac
    case "$key" in
      unhealthy_since) unhealthy_since="$value" ;;
      restart_count) restart_count="$value" ;;
      cooldown_until) cooldown_until="$value" ;;
    esac
  done <"$state_file"
}

save_state() {
  tmp_state_file="${state_file}.tmp.$$"
  cat >"$tmp_state_file" <<EOF
unhealthy_since=${unhealthy_since}
restart_count=${restart_count}
cooldown_until=${cooldown_until}
EOF
  mv "$tmp_state_file" "$state_file"
}

clear_state() {
  container_name="$1"
  rm -f "$(state_file_path "$container_name")"
}

detect_watch_project() {
  if [ -n "$WATCH_PROJECT" ]; then
    printf '%s\n' "$WATCH_PROJECT"
    return 0
  fi

  if [ -z "${HOSTNAME:-}" ]; then
    return 1
  fi

  project_name="$(
    docker inspect --type container \
      --format '{{ index .Config.Labels "'"$PROJECT_LABEL_KEY"'" }}' \
      "$HOSTNAME" 2>/dev/null || true
  )"
  project_name="$(strip_no_value "$project_name")"
  if [ -z "$project_name" ]; then
    return 1
  fi

  printf '%s\n' "$project_name"
}

list_target_containers() {
  if [ -n "$WATCH_PROJECT" ]; then
    docker ps -a \
      --filter "label=${PROJECT_LABEL_KEY}=${WATCH_PROJECT}" \
      --filter "label=${WATCH_LABEL_KEY}=${WATCH_LABEL_VALUE}" \
      --format '{{.Names}}'
    return 0
  fi

  for container_name in $WATCH_CONTAINERS; do
    printf '%s\n' "$container_name"
  done
}

container_running() {
  container_name="$1"
  docker inspect --type container \
    --format '{{.State.Running}}' \
    "$container_name" 2>/dev/null || true
}

container_health() {
  container_name="$1"
  docker inspect --type container \
    --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' \
    "$container_name" 2>/dev/null || true
}

compute_restart_cooldown() {
  attempt_count="$1"
  delay="$RESTART_BASE_COOLDOWN"
  step=1

  if [ "$delay" -gt "$RESTART_MAX_COOLDOWN" ]; then
    delay="$RESTART_MAX_COOLDOWN"
  fi

  while [ "$step" -lt "$attempt_count" ]; do
    if [ "$delay" -ge "$RESTART_MAX_COOLDOWN" ]; then
      delay="$RESTART_MAX_COOLDOWN"
      break
    fi

    next_delay=$((delay * 2))
    if [ "$next_delay" -gt "$RESTART_MAX_COOLDOWN" ]; then
      delay="$RESTART_MAX_COOLDOWN"
      break
    fi

    delay="$next_delay"
    step=$((step + 1))
  done

  printf '%s\n' "$delay"
}

restart_unhealthy_container() {
  container_name="$1"
  timestamp="$(date +%Y%m%d%H%M%S 2>/dev/null || date +%s)"
  log_file="${LOG_DIR}/${container_name}-${timestamp}.log"
  restart_rc=0

  {
    echo "=== ${container_name} unhealthy at ${timestamp} ==="
    echo "watch_project=${WATCH_PROJECT:-explicit-container-list}"
    echo
    echo "--- docker inspect health ---"
    docker inspect "$container_name" --format '{{json .State.Health}}' 2>&1 || true
    echo
    echo "--- docker logs tail (${LOG_TAIL}) ---"
    docker logs --tail "$LOG_TAIL" "$container_name" 2>&1 || true
    echo
    echo "--- docker restart ---"
    if docker restart "$container_name" 2>&1; then
      echo "restart_status=success"
    else
      restart_rc=$?
      echo "restart_status=failure exit_code=${restart_rc}"
    fi
  } >>"$log_file" 2>&1

  if [ "$restart_rc" -eq 0 ]; then
    log "Restarted [$container_name]; snapshot saved to $log_file"
  else
    log "Restart failed for [$container_name] (exit=${restart_rc}); snapshot saved to $log_file"
  fi

  return "$restart_rc"
}

process_container() {
  container_name="$1"
  now="$(date +%s)"
  running="$(container_running "$container_name")"

  if [ "$running" != "true" ]; then
    clear_state "$container_name"
    return 0
  fi

  health="$(container_health "$container_name")"
  load_state "$container_name"

  case "$health" in
    healthy)
      clear_state "$container_name"
      ;;
    unhealthy)
      if [ "$unhealthy_since" -eq 0 ]; then
        unhealthy_since="$now"
        save_state
        log "Container [$container_name] became unhealthy; waiting ${UNHEALTHY_GRACE_PERIOD}s before restart."
        return 0
      fi

      unhealthy_for=$((now - unhealthy_since))
      if [ "$unhealthy_for" -lt "$UNHEALTHY_GRACE_PERIOD" ]; then
        return 0
      fi

      if [ "$now" -lt "$cooldown_until" ]; then
        return 0
      fi

      restart_count=$((restart_count + 1))
      cooldown_seconds="$(compute_restart_cooldown "$restart_count")"
      cooldown_until=$((now + cooldown_seconds))
      unhealthy_since=0

      restart_unhealthy_container "$container_name" || true
      save_state
      log "Cooldown for [$container_name]: ${cooldown_seconds}s before the next auto-restart attempt."
      ;;
    *)
      if [ "$unhealthy_since" -ne 0 ]; then
        unhealthy_since=0
        save_state
      fi
      ;;
  esac
}

require_cmd docker
require_positive_int "$CHECK_INTERVAL" "CHECK_INTERVAL"
require_positive_int "$LOG_TAIL" "LOG_TAIL"
require_non_negative_int "$UNHEALTHY_GRACE_PERIOD" "UNHEALTHY_GRACE_PERIOD"
require_non_negative_int "$RESTART_BASE_COOLDOWN" "RESTART_BASE_COOLDOWN"
require_non_negative_int "$RESTART_MAX_COOLDOWN" "RESTART_MAX_COOLDOWN"

mkdir -p "$LOG_DIR" "$STATE_DIR"

WATCH_PROJECT="$(detect_watch_project || true)"
if [ -n "$WATCH_PROJECT" ]; then
  log "Watching compose project [$WATCH_PROJECT] for unhealthy labelled containers."
else
  log "Compose-project discovery unavailable; watching explicit containers: $WATCH_CONTAINERS"
fi

while true; do
  target_containers="$(list_target_containers)"
  if [ -n "$target_containers" ]; then
    printf '%s\n' "$target_containers" | while IFS= read -r container_name; do
      [ -n "$container_name" ] || continue
      process_container "$container_name"
    done
  fi

  sleep "$CHECK_INTERVAL"
done
