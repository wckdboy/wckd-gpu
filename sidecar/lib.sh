#!/usr/bin/env bash
# Shared rclone setup for the wckd pod sidecar.

wckd_log() {
  printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2
}

wckd_require() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    wckd_log "missing env ${name}"
    exit 1
  fi
}

wckd_rclone_env() {
  wckd_require WCKD_SESSION_ID
  wckd_require WCKD_PROJECT_ID
  wckd_require WCKD_S3_BUCKET
  wckd_require WCKD_S3_ENDPOINT
  wckd_require WCKD_S3_ACCESS_KEY_ID
  wckd_require WCKD_S3_SECRET_ACCESS_KEY

  export RCLONE_CONFIG_WCKD_TYPE=s3
  export RCLONE_CONFIG_WCKD_PROVIDER=Other
  export RCLONE_CONFIG_WCKD_ACCESS_KEY_ID="${WCKD_S3_ACCESS_KEY_ID}"
  export RCLONE_CONFIG_WCKD_SECRET_ACCESS_KEY="${WCKD_S3_SECRET_ACCESS_KEY}"
  export RCLONE_CONFIG_WCKD_ENDPOINT="${WCKD_S3_ENDPOINT}"
  export RCLONE_CONFIG_WCKD_REGION="${WCKD_S3_REGION:-auto}"
  export RCLONE_CONFIG_WCKD_NO_CHECK_BUCKET=true
  export RCLONE_CONFIG_WCKD_FORCE_PATH_STYLE=true

  if ! command -v rclone >/dev/null 2>&1; then
    wckd_log "installing rclone"
    curl -fsSL https://rclone.org/install.sh | bash
  fi
}

# Split "a|b;;c|d" and call the given function with left and right.
wckd_each_pair() {
  local spec="$1"
  local fn="$2"
  spec="${spec};;"
  while [[ "${spec}" == *";;"* ]]; do
    local pair="${spec%%;;*}"
    spec="${spec#*;;}"
    if [[ -z "${pair}" ]]; then
      continue
    fi
    local left="${pair%%|*}"
    local right="${pair#*|}"
    if [[ -z "${left}" || -z "${right}" || "${left}" == "${right}" ]]; then
      wckd_log "skipping malformed sync pair ${pair}"
      continue
    fi
    "${fn}" "${left}" "${right}"
  done
}
