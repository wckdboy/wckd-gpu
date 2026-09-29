#!/usr/bin/env bash
# Hydrate the project prefix, run the preset workload, and drain on deadline
# or when the CLI writes sessions/<id>/control.json with action=drain.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

wckd_rclone_env

WORKSPACE="${WCKD_WORKSPACE:-/workspace}"
mkdir -p "${WORKSPACE}"

hydrate_one() {
  local src="$1"
  local dest="$2"
  mkdir -p "${dest}"
  wckd_log "hydrate s3:${WCKD_S3_BUCKET}/${src} -> ${dest}"
  rclone sync "wckd:${WCKD_S3_BUCKET}/${src}" "${dest}"
}

HYDRATE_MAP="${WCKD_HYDRATE_MAP:-projects/${WCKD_PROJECT_ID}/workspace|${WORKSPACE}}"
wckd_each_pair "${HYDRATE_MAP}" hydrate_one

printf '{"session_id":"%s","at":"%s"}\n' "${WCKD_SESSION_ID}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  | rclone rcat "wckd:${WCKD_S3_BUCKET}/sessions/${WCKD_SESSION_ID}/hydrate-ok.json"

workload_pid=""
if [[ -n "${WCKD_WORKLOAD:-}" ]]; then
  wckd_log "starting workload: ${WCKD_WORKLOAD}"
  bash -lc "${WCKD_WORKLOAD}" &
  workload_pid=$!
  sleep 1
  if ! kill -0 "${workload_pid}" 2>/dev/null; then
    wckd_log "workload exited; holding the session until drain"
    sleep infinity &
    workload_pid=$!
  fi
else
  wckd_log "no WCKD_WORKLOAD; holding the session until drain"
  sleep infinity &
  workload_pid=$!
fi

deadline_epoch=""
if [[ -n "${WCKD_DEADLINE_AT:-}" ]]; then
  if ! deadline_epoch=$(date -u -d "${WCKD_DEADLINE_AT}" +%s); then
    wckd_log "could not parse WCKD_DEADLINE_AT=${WCKD_DEADLINE_AT}"
    deadline_epoch=""
  fi
fi

control_url="wckd:${WCKD_S3_BUCKET}/sessions/${WCKD_SESSION_ID}/control.json"

should_drain() {
  if [[ -n "${deadline_epoch}" ]]; then
    local now
    now=$(date -u +%s)
    if (( now >= deadline_epoch )); then
      return 0
    fi
  fi
  local body
  if body=$(rclone cat "${control_url}" 2>/dev/null); then
    if grep -q '"action"[[:space:]]*:[[:space:]]*"drain"' <<<"${body}"; then
      return 0
    fi
  fi
  return 1
}

draining=0
drain_and_exit() {
  if [[ "${draining}" == "1" ]]; then
    return
  fi
  draining=1
  wckd_log "draining session ${WCKD_SESSION_ID}"
  if [[ -n "${workload_pid}" ]]; then
    kill -TERM "${workload_pid}" 2>/dev/null || true
    local grace="${WCKD_DRAIN_GRACE_SEC:-20}"
    local i
    for ((i = 0; i < grace; i++)); do
      if ! kill -0 "${workload_pid}" 2>/dev/null; then
        break
      fi
      sleep 1
    done
    kill -KILL "${workload_pid}" 2>/dev/null || true
    wait "${workload_pid}" 2>/dev/null || true
  fi
  bash "${SCRIPT_DIR}/drain.sh"
  exit $?
}

trap drain_and_exit TERM INT

wckd_log "session ${WCKD_SESSION_ID} running until ${WCKD_DEADLINE_AT:-manual stop}"
while true; do
  if should_drain; then
    drain_and_exit
  fi
  sleep "${WCKD_POLL_SEC:-10}"
done
