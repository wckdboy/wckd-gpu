#!/usr/bin/env bash
# Push workspace paths back to the project prefix and write drain-ok.json.
# The CLI terminates the pod only after that object exists, unless --force.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

wckd_rclone_env

declare -a RCLONE_EXCLUDES=()
if [[ -n "${WCKD_DRAIN_EXCLUDES:-}" ]]; then
  IFS=',' read -r -a _excludes <<<"${WCKD_DRAIN_EXCLUDES}"
  for ex in "${_excludes[@]}"; do
    if [[ -n "${ex}" ]]; then
      RCLONE_EXCLUDES+=(--exclude "${ex}")
    fi
  done
fi

drain_one() {
  local src="$1"
  local dest="$2"
  if [[ ! -d "${src}" ]]; then
    wckd_log "drain source ${src} is missing; syncing an empty prefix"
    mkdir -p "${src}"
  fi
  wckd_log "drain ${src} -> s3:${WCKD_S3_BUCKET}/${dest}"
  rclone sync "${src}" "wckd:${WCKD_S3_BUCKET}/${dest}" --checksum "${RCLONE_EXCLUDES[@]}"
  rclone lsf -R "${src}" \
    | rclone rcat "wckd:${WCKD_S3_BUCKET}/sessions/${WCKD_SESSION_ID}/drain-files.txt" || true
}

DRAIN_MAP="${WCKD_DRAIN_MAP:-/workspace|projects/${WCKD_PROJECT_ID}/workspace}"
wckd_each_pair "${DRAIN_MAP}" drain_one

printf '{"session_id":"%s","at":"%s","ok":true}\n' "${WCKD_SESSION_ID}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  | rclone rcat "wckd:${WCKD_S3_BUCKET}/sessions/${WCKD_SESSION_ID}/drain-ok.json"

wckd_log "drain-ok written for ${WCKD_SESSION_ID}"
